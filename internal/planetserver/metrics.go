package planetserver

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/yomezza/cosmosimulator/internal/planet/access"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/platform/telemetry"
)

type transportMetrics struct{ packets, bytes, errors atomic.Uint64 }

// Aggregates only; the same live session and admin role are checked per request.
func (a *App) metricsView(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.config.DBTimeout)
	defer cancel()
	id, err := access.Authenticate(ctx, a.db, r)
	if err != nil {
		if errors.Is(err, access.ErrAuth) {
			httpkit.Fail(w, 401, "auth_required")
		} else {
			httpkit.Fail(w, 503, "runtime_unavailable")
		}
		return
	}
	var role string
	if err = a.db.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, id.User).Scan(&role); err != nil {
		httpkit.Fail(w, 503, "runtime_unavailable")
		return
	}
	if role != "admin" {
		httpkit.Fail(w, 403, "access_denied")
		return
	}
	if a.motion == nil || a.scenes == nil {
		httpkit.Fail(w, 503, "runtime_unavailable")
		return
	}
	a.wsMu.Lock()
	connections := a.connections
	a.wsMu.Unlock()
	httpkit.Reply(w, 200, map[string]any{"service": "planet", "version": Version, "process": telemetry.Process(a.db), "connections": connections, "movement": a.motion.Metrics(), "scenes": a.scenes.Metrics(), "checkpoints": a.checkpoints.Snapshot(), "checkpoint_errors": a.checkpointErrors.Load(), "transport": map[string]any{"packets": a.transport.packets.Load(), "json_bytes": a.transport.bytes.Load(), "write_errors": a.transport.errors.Load()}})
}
