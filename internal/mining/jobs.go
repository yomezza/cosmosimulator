package mining

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/events"
)

type MiningJob struct {
	Resource   string    `json:"resource"`
	Quantity   int       `json:"quantity"`
	FinishTick int64     `json:"finish_tick"`
	StartedAt  time.Time `json:"started_at"`
	Planet     int       `json:"planet"`
}

func CompleteMining(ctx context.Context, tx pgx.Tx, tick int64) error {
	rows, err := tx.Query(ctx, `SELECT user_id,ship_id,resource,quantity,automation_id FROM mining_jobs WHERE finish_tick<=$1 ORDER BY user_id`, tick)
	if err != nil {
		return err
	}
	type result struct {
		Auto     *int64
		id       int64
		ship     int64
		resource string
		q        int
	}
	jobs := []result{}
	for rows.Next() {
		var j result
		if err = rows.Scan(&j.id, &j.ship, &j.resource, &j.q, &j.Auto); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if err = events.RecordEvent(ctx, tx, j.id, "mining_complete", map[string]any{"resource": j.resource, "quantity": j.q, "ship": j.ship}); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE ships SET cargo=jsonb_set(cargo,ARRAY[$2::text],to_jsonb(COALESCE((cargo->>$2)::int,0)+$3)) WHERE id=$1`, j.ship, j.resource, j.q); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET xp=xp+5 WHERE id=$1`, j.id); err != nil {
			return err
		}
		if j.Auto != nil {
			if _, err = tx.Exec(ctx, `UPDATE automation_tasks SET progress=progress+$2,carried=carried+$2,pending=0 WHERE id=$1`, *j.Auto, j.q); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `DELETE FROM mining_jobs WHERE ship_id=$1`, j.ship); err != nil {
			return err
		}
	}
	return nil
}
