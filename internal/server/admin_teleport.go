package server

import (
	"context"
	"encoding/json"
	"net/http"
)

func (a *App) adminTeleport(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	var in Command
	if !decode(w, r, &in) {
		return
	}
	g, err := a.loadUniverse(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	if in.To < 1 || in.To > len(g.Stars) || g.Stars[in.To-1].Deleted {
		fail(w, 400, "Некорректная система")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		a.internal(w, err)
		return
	}
	// Serialize with navigation and re-check the role under the user lock.
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&role); err != nil {
		a.internal(w, err)
		return
	}
	if role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	s, err := ensureShip(ctx, tx, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if in.Ship > 0 {
		var owner int64
		if err = tx.QueryRow(ctx, `SELECT user_id FROM ships WHERE id=$1`, in.Ship).Scan(&owner); err != nil || owner != u.ID {
			fail(w, 404, "Корабль не найден")
			return
		}
		s, err = readShip(ctx, tx, in.Ship)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	if busy, e := automationBusy(ctx, tx, s.ID); e != nil {
		a.internal(w, e)
		return
	} else if busy {
		fail(w, 409, "Приостановите задачу планировщика")
		return
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mining_jobs WHERE ship_id=$1) OR EXISTS(SELECT 1 FROM deliveries WHERE ship_id=$1 AND status='carrying')`, s.ID).Scan(&busy); err != nil {
		a.internal(w, err)
		return
	}
	if busy {
		fail(w, 409, "Завершите добычу или доставку перед телепортацией")
		return
	}
	// Keep survey progress for later resumption, but stop scanning remotely.
	if _, err = tx.Exec(ctx, `UPDATE research_jobs SET paused=TRUE WHERE ship_id=$1`, s.ID); err != nil {
		a.internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE ships SET star=$2,object_id='entry',position_mode='approach',destination=NULL,arrival=NULL,departure=NULL,travel_path='[]',local_target=NULL,local_mode=NULL,local_started=NULL,local_arrival=NULL,waypoints='[]',reserved_fuel=0 WHERE id=$1`, s.ID, in.To); err != nil {
		a.internal(w, err)
		return
	}
	raw, _ := json.Marshal(map[string]any{"ship": s.ID, "from": s.Star, "to": in.To})
	if _, err = tx.Exec(ctx, `INSERT INTO admin_log(actor,action,data) VALUES($1,'teleport',$2)`, u.ID, raw); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
