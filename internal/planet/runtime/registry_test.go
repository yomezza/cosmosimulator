package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yomezza/cosmosimulator/internal/geography"
	"github.com/yomezza/cosmosimulator/internal/planet/scene"
)

func registryScene(t *testing.T) *scene.Scene {
	t.Helper()
	g := &geography.PlanetSurface{Width: 4, Height: 4, GridW: 2, GridH: 2, SeaLevel: 460, Heights: []int{700, 700, 700, 700}, Water: make([]int, 4), Sectors: []geography.PlanetSector{{Number: 1, Center: geography.SurfacePoint{X: 2, Y: 2}}}}
	s, err := scene.Generate(context.Background(), scene.Key{Surface: 1, Sector: 1}, g, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestRegistrySingleFlightPinsAndRetirement(t *testing.T) {
	s := registryScene(t)
	var calls, validations atomic.Int64
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	r, err := NewRegistry(context.Background(), 1, time.Second, time.Second, func(ctx context.Context, k scene.Key) (*scene.Scene, error) {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-gate:
			return s, nil
		}
	}, func(context.Context, *scene.Scene) error { validations.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())
	const clients = 24
	var wg sync.WaitGroup
	results := make(chan *Pin, clients)
	failures := make(chan error, clients)
	for range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := r.Acquire(context.Background(), s.Key())
			if err != nil {
				failures <- err
			} else {
				results <- p
			}
		}()
	}
	<-started
	close(gate)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if calls.Load() != 1 || validations.Load() != clients {
		t.Fatal("load not single-flight or cached entry skipped fresh validation")
	}
	if _, err := r.Acquire(context.Background(), scene.Key{Surface: 2, Sector: 1}); !errors.Is(err, ErrCapacity) {
		t.Fatal("unbounded registry")
	}
	r.Evict(time.Now().Add(time.Hour))
	if r.Count() != 1 {
		t.Fatal("eviction discarded a pinned scene")
	}
	if r.DrainSurface(1) {
		t.Fatal("retirement discarded active pins")
	}
	for p := range results {
		if p.Scene() != s {
			t.Fatal("pins do not share scene")
		}
		p.Release()
		p.Release()
	}
	if !r.DrainSurface(1) || r.Count() != 0 {
		t.Fatal("drained scene not evicted")
	}
	if _, err := r.Acquire(context.Background(), s.Key()); !errors.Is(err, ErrClosed) {
		t.Fatal("retired surface reopened")
	}
}
func TestRegistryCancellationRetryAndIdle(t *testing.T) {
	s := registryScene(t)
	var attempts atomic.Int64
	gate := make(chan struct{})
	start := make(chan struct{})
	r, _ := NewRegistry(context.Background(), 2, time.Second, time.Second, func(ctx context.Context, k scene.Key) (*scene.Scene, error) {
		if attempts.Add(1) == 1 {
			close(start)
			<-gate
			return nil, errors.New("temporary database failure")
		}
		return s, nil
	}, func(context.Context, *scene.Scene) error { return nil })
	defer r.Close(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := r.Acquire(ctx, s.Key()); result <- err }()
	<-start
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(gate)
	// Wait for the failed single-flight, using its completion signal rather than sleep.
	r.mu.Lock()
	e := r.entries[s.Key()]
	r.mu.Unlock()
	if e != nil {
		<-e.done
	}
	p, err := r.Acquire(context.Background(), s.Key())
	if err != nil {
		t.Fatal(err)
	}
	p.Release()
	r.Evict(time.Now().Add(2 * time.Second))
	if r.Count() != 0 {
		t.Fatal("idle snapshot remained loaded")
	}
	p, err = r.Acquire(context.Background(), s.Key())
	if err != nil {
		t.Fatal(err)
	}
	p.Release()
	if attempts.Load() != 3 {
		t.Fatal("failed/evicted scene was not reloaded")
	}
	if err = r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Acquire(context.Background(), s.Key()); !errors.Is(err, ErrClosed) {
		t.Fatal("shutdown allowed load")
	}
}
func TestRegistryStopCancelsPreparation(t *testing.T) {
	start := make(chan struct{})
	r, _ := NewRegistry(context.Background(), 1, time.Second, time.Minute, func(ctx context.Context, k scene.Key) (*scene.Scene, error) {
		close(start)
		<-ctx.Done()
		return nil, ctx.Err()
	}, func(context.Context, *scene.Scene) error { return nil })
	result := make(chan error, 1)
	go func() { _, err := r.Acquire(context.Background(), scene.Key{Surface: 1, Sector: 1}); result <- err }()
	<-start
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal("shutdown did not cancel generator", err)
	}
	if err := <-result; err == nil {
		t.Fatal("cancelled preparation returned a scene")
	}
}
