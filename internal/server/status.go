package server

import (
	"context"
	"net/http"
	"time"
)

// Public aggregate information contains no account names or private activity.
func (a *App) serverStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var players, bots int
	err := a.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE NOT is_bot AND virtual_run IS NULL AND last_active>now()-interval '5 minutes'),count(*) FILTER(WHERE is_bot AND virtual_run IS NULL) FROM users`).Scan(&players, &bots)
	if err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]any{"name": "Иггдрасиль", "players": players, "bots": bots})
}
