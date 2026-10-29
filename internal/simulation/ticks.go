package simulation

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/colonies"
	"github.com/yomezza/cosmosimulator/internal/commerce"
	"github.com/yomezza/cosmosimulator/internal/mining"
)

func (a *Service) Simulate(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.Activities(ctx); err != nil && ctx.Err() == nil {
				log.Printf("activities: %v", err)
			}
			if err := a.Tick(ctx); err != nil && ctx.Err() == nil {
				log.Printf("simulation: %v", err)
			}
		}
	}
}

func (a *Service) Tick(ctx context.Context) error { return a.AdvanceTick(ctx, false) }

func (a *Service) AdvanceTick(ctx context.Context, force bool, actor ...int64) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		return err
	}
	var tick int64
	var seconds int
	err = tx.QueryRow(ctx, `UPDATE world w SET tick=tick+1,pulses=pulses+g.date_step,last_tick=now() FROM game_settings g WHERE w.id=1 AND g.id=1 AND ($1 OR last_tick<=now()-make_interval(secs=>g.tick_seconds)) RETURNING tick,g.tick_seconds`, force).Scan(&tick, &seconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET ore=ore+1 WHERE NOT is_bot`); err != nil {
		return err
	}
	if force {
		if _, err = tx.Exec(ctx, `UPDATE research_jobs SET next_at=next_at-make_interval(secs=>$1) WHERE NOT paused`, seconds); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE ships SET arrival=arrival-make_interval(secs=>$1),departure=departure-make_interval(secs=>$1),local_arrival=local_arrival-make_interval(secs=>$1),local_started=local_started-make_interval(secs=>$1) WHERE arrival IS NOT NULL OR local_arrival IS NOT NULL`, seconds); err != nil {
			return err
		}
	}
	if err = colonies.AdvanceColonies(ctx, tx, tick); err != nil {
		return err
	}
	if err = mining.CompleteMining(ctx, tx, tick); err != nil {
		return err
	}
	if tick%3 == 0 {
		if err = commerce.TradeBots(ctx, tx); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`); err != nil {
		return err
	}
	if force && len(actor) > 0 {
		raw, _ := json.Marshal(map[string]any{"tick": tick})
		if _, err = tx.Exec(ctx, `INSERT INTO admin_log(actor,action,data) VALUES($1,'tick',$2)`, actor[0], raw); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
