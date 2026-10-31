package movement

import "github.com/yomezza/cosmosimulator/internal/platform/telemetry"

type observations struct {
	lag, paths, queue                             telemetry.Samples
	queued, rejected, budget, canceled, coalesced uint64 // guarded by Hub.mu
}

func (h *Hub) Metrics() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return map[string]any{"sectors": len(h.actors), "actors": len(h.members), "path_queued": h.metrics.queued, "path_rejected": h.metrics.rejected, "path_budget": h.metrics.budget, "path_canceled": h.metrics.canceled, "mailbox_coalesced": h.metrics.coalesced, "tick_lag": h.metrics.lag.Snapshot(), "path_compute": h.metrics.paths.Snapshot(), "path_queue_wait": h.metrics.queue.Snapshot()}
}
