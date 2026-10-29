package worldstore

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	tradecatalog "github.com/yomezza/cosmosimulator/internal/commerce/catalog"
	"github.com/yomezza/cosmosimulator/internal/universe"
)

func EnsureStations(ctx context.Context, tx pgx.Tx, g *universe.Galaxy) error {
	founders := map[string]int64{}
	rows, err := tx.Query(ctx, `SELECT id,founder_key FROM companies WHERE founder_key IN('founder-01','founder-02') ORDER BY founder_key`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var key string
		if err = rows.Scan(&id, &key); err != nil {
			rows.Close()
			return err
		}
		founders[key] = id
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	core := map[int]bool{}
	for _, d := range g.Catalog {
		for _, id := range universe.CoreSystems(g, d.ID) {
			core[id] = true
		}
	}
	if len(g.Catalog) == 0 {
		for _, id := range universe.CoreSystems(g) {
			core[id] = true
		}
	}
	list := []map[string]any{}
	for _, star := range g.Stars {
		if star.Deleted {
			continue
		}
		sites := universe.SystemSites(star)
		if star.Ports == nil && core[star.ID] && !universe.HasStation(star.ID) {
			sites = append(sites, universe.Site{ID: "station", Name: star.Name + " · CORE", Kind: "Орбитальная станция", Orbit: 1})
		}
		for _, s := range sites {
			if s.Kind != "Орбитальная станция" {
				continue
			}
			spec := tradecatalog.StationProfile(star.ID, s.ID)
			var founder *int64
			if x, ok := founders[star.Founder]; ok {
				founder = &x
			}
			yard := star.ID == 1 || star.ID == 5 || star.ID%17 == 0
			list = append(list, map[string]any{"star": star.ID, "object_id": s.ID, "name": s.Name, "specialization": spec, "founder": founder, "shipyard": yard, "ship_stock": 2})
		}
	}
	raw, _ := json.Marshal(list)
	_, err = tx.Exec(ctx, `INSERT INTO stations(star,object_id,name,specialization,founder,shipyard,ship_stock) SELECT * FROM jsonb_to_recordset($1) AS x(star int,object_id text,name text,specialization text,founder bigint,shipyard bool,ship_stock int) ON CONFLICT(star,object_id) DO UPDATE SET founder=EXCLUDED.founder WHERE stations.company_id IS NULL`, raw)
	if err != nil {
		return err
	}
	return tradecatalog.SeedCommerce(ctx, tx)
}
