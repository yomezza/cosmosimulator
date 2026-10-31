package server

import (
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/platform/telemetry"
	"net/http"
)

func (a *App) metricsView(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	httpkit.Reply(w, 200, map[string]any{"service": "cosmic", "version": Version, "process": telemetry.Process(a.DB), "ticks": a.Simulation.TickDurations.Snapshot(), "simulation_loop_lag": a.Simulation.TickLoopLag.Snapshot(), "tick_errors": a.Simulation.TickErrors.Load()})
}
