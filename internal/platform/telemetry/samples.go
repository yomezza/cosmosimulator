// Package telemetry provides bounded, aggregate observations without labels,
// SQL text, account IDs or background workers. Zero values are ready for use.
package telemetry

import (
	"slices"
	"sync"
	"time"
)

const WindowSize = 2048

type Samples struct {
	mu             sync.Mutex
	values         [WindowSize]time.Duration
	count          uint64
	total, maximum time.Duration
}

type Distribution struct {
	Count  uint64  `json:"count"`
	Window int     `json:"window_samples"`
	P50    float64 `json:"p50_ms"`
	P95    float64 `json:"p95_ms"`
	P99    float64 `json:"p99_ms"`
	Max    float64 `json:"max_ms"`
	Total  float64 `json:"total_ms"`
}

func (s *Samples) Observe(d time.Duration) {
	d = max(0, d)
	s.mu.Lock()
	s.values[s.count%WindowSize] = d
	s.count++
	s.total += d
	s.maximum = max(s.maximum, d)
	s.mu.Unlock()
}

func (s *Samples) Snapshot() Distribution {
	s.mu.Lock()
	n := min(s.count, WindowSize)
	v := append([]time.Duration(nil), s.values[:n]...)
	out := Distribution{Count: s.count, Window: int(n), Max: float64(s.maximum) / float64(time.Millisecond), Total: float64(s.total) / float64(time.Millisecond)}
	s.mu.Unlock()
	slices.Sort(v)
	quantile := func(p int) float64 {
		if len(v) == 0 {
			return 0
		}
		return float64(v[max(0, (len(v)*p+99)/100-1)]) / float64(time.Millisecond)
	}
	out.P50, out.P95, out.P99 = quantile(50), quantile(95), quantile(99)
	return out
}
