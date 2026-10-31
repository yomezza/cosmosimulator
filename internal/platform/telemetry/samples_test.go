package telemetry

import (
	"sync"
	"testing"
	"time"
)

func TestConcurrentBoundedObservations(t *testing.T) {
	var s Samples
	if v := s.Snapshot(); v.Count != 0 || v.P99 != 0 {
		t.Fatal(v)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				s.Observe(2 * time.Millisecond)
				_ = s.Snapshot()
			}
		}()
	}
	wg.Wait()
	v := s.Snapshot()
	if v.Count != 8000 || v.Window != 2048 || v.P50 != 2 || v.P95 != 2 || v.P99 != 2 || v.Total != 16000 {
		t.Fatal(v)
	}
}
