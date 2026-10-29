package inventory

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func Warehouse(ctx context.Context, tx pgx.Tx, uid int64, star int, station string) (map[string]int, error) {
	_, err := tx.Exec(ctx, `INSERT INTO warehouses(user_id,star,station) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, uid, star, station)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT cargo FROM warehouses WHERE user_id=$1 AND star=$2 AND station=$3 FOR UPDATE`, uid, star, station).Scan(&raw)
	if err != nil {
		return nil, err
	}
	m := map[string]int{}
	err = json.Unmarshal(raw, &m)
	return m, err
}

func PutWarehouse(ctx context.Context, tx pgx.Tx, uid int64, star int, station string, m map[string]int) error {
	raw, _ := json.Marshal(m)
	_, err := tx.Exec(ctx, `UPDATE warehouses SET cargo=$4 WHERE user_id=$1 AND star=$2 AND station=$3`, uid, star, station, raw)
	return err
}
