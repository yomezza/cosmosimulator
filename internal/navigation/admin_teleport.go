package navigation

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/yomezza/cosmosimulator/internal/commands"
	"github.com/yomezza/cosmosimulator/internal/fleet"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
)

func (a *Service) AdminTeleport(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	var in commands.Command
	if !httpkit.Decode(w, r, &in) {
		return
	}
	g, err := a.World.Load(r.Context())
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if in.To < 1 || in.To > len(g.Stars) || g.Stars[in.To-1].Deleted {
		httpkit.Fail(w, 400, "Некорректная система")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		httpkit.Internal(w, err)
		return
	}
	// Serialize with navigation and re-check the role under the user lock.
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&role); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	s, err := fleet.EnsureShip(ctx, tx, u.ID)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if in.Ship > 0 {
		var owner int64
		if err = tx.QueryRow(ctx, `SELECT user_id FROM ships WHERE id=$1`, in.Ship).Scan(&owner); err != nil || owner != u.ID {
			httpkit.Fail(w, 404, "Корабль не найден")
			return
		}
		s, err = fleet.ReadShip(ctx, tx, in.Ship)
		if err != nil {
			httpkit.Internal(w, err)
			return
		}
	}
	if e := fleet.Available(ctx, tx, s.ID); e != nil {
		httpkit.Fail(w, 409, e.Error())
		return
	}
	if busy, e := fleet.AutomationBusy(ctx, tx, s.ID); e != nil {
		httpkit.Internal(w, e)
		return
	} else if busy {
		httpkit.Fail(w, 409, "Приостановите задачу планировщика")
		return
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mining_jobs WHERE ship_id=$1) OR EXISTS(SELECT 1 FROM deliveries WHERE ship_id=$1 AND status='carrying')`, s.ID).Scan(&busy); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if busy {
		httpkit.Fail(w, 409, "Завершите добычу или доставку перед телепортацией")
		return
	}
	// Keep survey progress for later resumption, but stop scanning remotely.
	if _, err = tx.Exec(ctx, `UPDATE research_jobs SET paused=TRUE WHERE ship_id=$1`, s.ID); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE ships SET star=$2,object_id='entry',position_mode='approach',destination=NULL,arrival=NULL,departure=NULL,travel_path='[]',local_target=NULL,local_mode=NULL,local_started=NULL,local_arrival=NULL,waypoints='[]',reserved_fuel=0 WHERE id=$1`, s.ID, in.To); err != nil {
		httpkit.Internal(w, err)
		return
	}
	raw, _ := json.Marshal(map[string]any{"ship": s.ID, "from": s.Star, "to": in.To})
	if _, err = tx.Exec(ctx, `INSERT INTO admin_log(actor,action,data) VALUES($1,'teleport',$2)`, u.ID, raw); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]bool{"ok": true})
}
