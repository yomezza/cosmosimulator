package terminal

import (
	"context"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/fleet"
	"github.com/yomezza/cosmosimulator/internal/gameplay"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/universe"
	"github.com/yomezza/cosmosimulator/internal/worldstore"
)

func (a *Service) Estimate(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	to, e := strconv.Atoi(r.URL.Query().Get("to"))
	g, err := a.World.Load(r.Context())
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if e != nil || to < 1 || to > len(g.Stars) {
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
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, u.ID); err != nil {
		httpkit.Internal(w, err)
		return
	}
	s, err := fleet.EnsureShip(ctx, tx, u.ID)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	path := g.Path(s.Star, to)
	settings, err := gameplay.ReadGameplay(ctx, tx)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	sites, err := worldstore.AddCustomSites(ctx, tx, g.Stars[to-1], universe.SystemSites(g.Stars[to-1]))
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	var scanned int
	err = tx.QueryRow(ctx, `SELECT scanned FROM research_jobs WHERE user_id=$1 AND star=$2`, u.ID, to).Scan(&scanned)
	if err != nil && err != pgx.ErrNoRows {
		httpkit.Internal(w, err)
		return
	}
	hasJob := err == nil
	var known bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM surveys WHERE user_id=$1 AND star=$2)`, u.ID, to).Scan(&known); err != nil {
		httpkit.Internal(w, err)
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
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]any{"path": path, "objects": len(sites), "remaining": remaining, "scan_seconds": remaining * settings.ScanSeconds, "travel_seconds": jumps * s.Hull().JumpSeconds, "fuel": jumps * s.Hull().JumpFuel, "return_fuel": jumps * s.Hull().JumpFuel * 2, "cost": cost, "can_depart": len(path) > 0 && s.Fuel-s.ReservedFuel >= jumps*s.Hull().JumpFuel && u.Credits >= int64(cost), "known": known})
}
