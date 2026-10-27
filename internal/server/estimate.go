package server

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
)

func (a *App) estimate(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	to, e := strconv.Atoi(r.URL.Query().Get("to"))
	g, err := a.loadUniverse(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	if e != nil || to < 1 || to > len(g.Stars) {
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
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, u.ID); err != nil {
		a.internal(w, err)
		return
	}
	s, err := ensureShip(ctx, tx, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	path := g.path(s.Star, to)
	settings, err := readGameplay(ctx, tx)
	if err != nil {
		a.internal(w, err)
		return
	}
	sites, err := addCustomSites(ctx, tx, g.Stars[to-1], systemSites(g.Stars[to-1]))
	if err != nil {
		a.internal(w, err)
		return
	}
	var scanned int
	err = tx.QueryRow(ctx, `SELECT scanned FROM research_jobs WHERE user_id=$1 AND star=$2`, u.ID, to).Scan(&scanned)
	if err != nil && err != pgx.ErrNoRows {
		a.internal(w, err)
		return
	}
	hasJob := err == nil
	var known bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM surveys WHERE user_id=$1 AND star=$2)`, u.ID, to).Scan(&known); err != nil {
		a.internal(w, err)
		return
	}
	remaining := max(0, len(sites)-scanned)
	if known {
		remaining = 0
	}
	cost := 10
	if known || hasJob {
		cost = 0
	}
	jumps := max(0, len(path)-1)
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]any{"path": path, "objects": len(sites), "remaining": remaining, "scan_seconds": remaining * settings.ScanSeconds, "travel_seconds": jumps * s.hull().JumpSeconds, "fuel": jumps * s.hull().JumpFuel, "return_fuel": jumps * s.hull().JumpFuel * 2, "cost": cost, "can_depart": len(path) > 0 && s.Fuel-s.ReservedFuel >= jumps*s.hull().JumpFuel && u.Credits >= int64(cost), "known": known})
}
