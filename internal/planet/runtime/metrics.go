package runtime

func (r *Registry) Metrics() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	pins, loading := 0, 0
	for _, e := range r.entries {
		pins += e.refs
		select {
		case <-e.done:
		default:
			loading++
		}
	}
	return map[string]any{"cached": len(r.entries), "pins": pins, "loading": loading, "cache_hits": r.hits, "evicted": r.evicted, "cold_load": r.loads.Snapshot()}
}
