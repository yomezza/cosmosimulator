package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func databaseConfig(dsn, schema string) (*pgxpool.Config, string, error) {
	if schema == "" {
		schema = "public"
	}
	// PostgreSQL expands "$user" in search_path even when quoted.
	// PostgreSQL truncates identifiers beyond 63 bytes. Reject them rather than
	// accidentally using another application's schema. A schema is one identifier,
	// never a comma-separated search path or a SQL expression.
	if !utf8.ValidString(schema) || len(schema) > 63 || strings.ContainsRune(schema, 0) || schema == "$user" || schema == "information_schema" || strings.HasPrefix(strings.ToLower(schema), "pg_") {
		return nil, "", fmt.Errorf("DB_SCHEMA must be a valid PostgreSQL schema name (1–63 bytes), not a system schema")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, "", err
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	// Applied by PostgreSQL to every physical pool connection, including reconnects.
	// Deliberately omit public and $user to avoid falling back to unrelated tables.
	config.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{schema}.Sanitize()
	return config, schema, nil
}

func openDatabase(ctx context.Context, dsn, schema string) (*pgxpool.Pool, error) {
	config, schema, err := databaseConfig(dsn, schema)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	ready := false
	defer func() {
		if !ready {
			db.Close()
		}
	}()
	if err = db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("PostgreSQL connection: %w", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(784216)"); err != nil {
		return nil, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname=$1)`, schema).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		if _, err = tx.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
			return nil, fmt.Errorf("create DB_SCHEMA %q (create it manually or grant CREATE on the database): %w", schema, err)
		}
	}
	sql, err := assets.ReadFile("schema.sql")
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, string(sql)); err != nil {
		return nil, fmt.Errorf("initialize DB_SCHEMA %q (USAGE and CREATE on the schema are required): %w", schema, err)
	}
	upgrade, e := assets.ReadFile("schema033.sql")
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, string(upgrade)); e != nil {
		return nil, fmt.Errorf("upgrade 0.3.3: %w", e)
	}
	var galaxyExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM galaxies WHERE name='Edem')`).Scan(&galaxyExists); err != nil {
		return nil, err
	}
	if !galaxyExists {
		data, err := json.Marshal(generateGalaxy(1000, 20261026))
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO galaxies(name,data) VALUES('Edem',$1)`, data); err != nil {
			return nil, err
		}
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT data FROM galaxies WHERE name='Edem'`).Scan(&raw); err != nil {
		return nil, err
	}
	var existing Galaxy
	if err = json.Unmarshal(raw, &existing); err != nil {
		return nil, err
	}
	if existing.Version < 3 {
		enrichGalaxy(&existing)
		upgradeGeology(&existing)
		existing.Regions = regions
		raw, err = json.Marshal(existing)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE galaxies SET data=$1 WHERE name='Edem'`, raw); err != nil {
			return nil, err
		}
	}
	oldNames := make(map[int]string, len(existing.Stars))
	for _, star := range existing.Stars {
		oldNames[star.ID] = star.Name
	}
	if diversifySystemNames(&existing) {
		renames := []map[string]any{}
		for _, star := range existing.Stars {
			if oldNames[star.ID] != star.Name {
				renames = append(renames, map[string]any{"id": star.ID, "old": oldNames[star.ID], "name": star.Name})
			}
		}
		labels, _ := json.Marshal(renames)
		if _, err = tx.Exec(ctx, `UPDATE stations s SET name=m.name||substr(s.name,length(m.old)+1) FROM jsonb_to_recordset($1) AS m(id int,old text,name text) WHERE s.star=m.id AND s.name IN(m.old||' · PORT 1',m.old||' · PORT 2',m.old||' · PORT 3',m.old||' · CORE')`, labels); err != nil {
			return nil, err
		}

		raw, err = json.Marshal(existing)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE galaxies SET data=$1 WHERE name='Edem'`, raw); err != nil {
			return nil, err
		}
	}
	// Complete metadata for a v0.3.1 flight without changing its route or ETA.
	rows, err := tx.Query(ctx, `SELECT user_id,star,destination,arrival FROM ships WHERE destination IS NOT NULL AND jsonb_array_length(travel_path)<2`)
	if err != nil {
		return nil, err
	}
	type oldFlight struct {
		id       int64
		star, to int
		arrival  time.Time
	}
	flights := []oldFlight{}
	for rows.Next() {
		var f oldFlight
		if err = rows.Scan(&f.id, &f.star, &f.to, &f.arrival); err != nil {
			rows.Close()
			return nil, err
		}
		flights = append(flights, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, f := range flights {
		path := existing.path(f.star, f.to)
		raw, _ := json.Marshal(path)
		departure := f.arrival.Add(-time.Duration(max(0, len(path)-1)) * 5 * time.Second)
		if _, err = tx.Exec(ctx, `UPDATE ships SET travel_path=$2,departure=$3 WHERE user_id=$1`, f.id, raw, departure); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	ready = true
	return db, nil
}
