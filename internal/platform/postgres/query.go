package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func QueryMaps(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToMap)
}
