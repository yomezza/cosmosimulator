package events

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func RecordEvent(ctx context.Context, tx pgx.Tx, id int64, kind string, details any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO game_events(user_id,kind,data) VALUES($1,$2,$3)`, id, kind, raw)
	return err
}
