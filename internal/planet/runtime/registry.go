package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/yomezza/cosmosimulator/internal/planet/scene"
	"github.com/yomezza/cosmosimulator/internal/platform/telemetry"
)

var ErrClosed = errors.New("sector registry is stopped or surface is retiring")
var ErrCapacity = errors.New("sector registry capacity reached")

type Loader func(context.Context, scene.Key) (*scene.Scene, error)
type Validator func(context.Context, *scene.Scene) error
type entry struct {
	done          chan struct{}
	cancel        context.CancelFunc
	scene         *scene.Scene
	err           error
	refs, waiters int
	idle          time.Time
}

// Registry single-flights cold loads and pins immutable snapshots. Empty scenes
// have no simulation goroutine. The caller drives eviction/retirement explicitly.
type Registry struct {
	loads           telemetry.Samples
	hits, evicted   uint64
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	load            Loader
	validate        Validator
	capacity        int
	idle, timeLimit time.Duration
	entries         map[scene.Key]*entry
	retiring        map[int64]bool
	closed          bool
	wg              sync.WaitGroup
	finished        chan struct{}
	parallel        chan struct{}
}

func NewRegistry(ctx context.Context, capacity int, idle, timeLimit time.Duration, load Loader, validate Validator) (*Registry, error) {
	if capacity < 1 || capacity > 1024 || idle <= 0 || timeLimit <= 0 || load == nil || validate == nil {
		return nil, errors.New("invalid sector registry configuration")
	}
	run, cancel := context.WithCancel(ctx)
	return &Registry{ctx: run, cancel: cancel, load: load, validate: validate, capacity: capacity, idle: idle, timeLimit: timeLimit, entries: map[scene.Key]*entry{}, retiring: map[int64]bool{}, finished: make(chan struct{}), parallel: make(chan struct{}, 2)}, nil
}

type Pin struct {
	registry *Registry
	key      scene.Key
	entry    *entry
	once     sync.Once
}

func (p *Pin) Scene() *scene.Scene { return p.entry.scene }
func (p *Pin) Release() {
	p.once.Do(func() {
		r := p.registry
		r.mu.Lock()
		defer r.mu.Unlock()
		p.entry.refs--
		if p.entry.refs == 0 {
			p.entry.idle = time.Now()
		}
	})
}

func (r *Registry) Acquire(ctx context.Context, key scene.Key) (*Pin, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.closed || r.ctx.Err() != nil || r.retiring[key.Surface] {
		r.mu.Unlock()
		return nil, ErrClosed
	}
	e := r.entries[key]
	if e == nil {
		if len(r.entries) >= r.capacity {
			r.evictLocked(time.Now())
			if len(r.entries) >= r.capacity {
				r.mu.Unlock()
				return nil, ErrCapacity
			}
		}
		loadCtx, cancel := context.WithTimeout(r.ctx, r.timeLimit)
		e = &entry{done: make(chan struct{}), cancel: cancel}
		r.entries[key] = e
		r.wg.Add(1)
		go r.prepare(loadCtx, key, e)
	}
	if e.scene != nil {
		r.hits++
	}
	e.waiters++
	r.mu.Unlock()
	select {
	case <-ctx.Done():
		r.mu.Lock()
		e.waiters--
		e.idle = time.Now()
		r.mu.Unlock()
		return nil, ctx.Err()
	case <-e.done:
	}
	r.mu.Lock()
	e.waiters--
	if ctx.Err() != nil {
		e.idle = time.Now()
		r.mu.Unlock()
		return nil, ctx.Err()
	}
	if r.closed || r.retiring[key.Surface] {
		r.mu.Unlock()
		return nil, ErrClosed
	}
	if e.err != nil {
		err := e.err
		r.mu.Unlock()
		return nil, err
	}
	e.refs++
	r.mu.Unlock()
	p := &Pin{registry: r, key: key, entry: e}
	if err := r.validate(ctx, e.scene); err != nil {
		p.Release()
		return nil, err
	}
	// Shutdown/retirement may have started during the fresh DB validation.
	r.mu.Lock()
	unavailable := r.closed || r.ctx.Err() != nil || r.retiring[key.Surface]
	r.mu.Unlock()
	if unavailable {
		p.Release()
		return nil, ErrClosed
	}
	return p, nil
}

func (r *Registry) prepare(ctx context.Context, key scene.Key, e *entry) {
	defer r.wg.Done()
	defer e.cancel()
	began := time.Now()
	defer func() { r.loads.Observe(time.Since(began)) }()
	var s *scene.Scene
	var err error
	select {
	case r.parallel <- struct{}{}:
		s, err = r.load(ctx, key)
		<-r.parallel
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err == nil && (s == nil || s.Key() != key) {
		err = scene.ErrCorrupt
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e.scene, e.err, e.idle = s, err, time.Now()
	if err != nil && r.entries[key] == e {
		delete(r.entries, key)
	}
	close(e.done)
}

func (r *Registry) evictLocked(now time.Time) {
	for key, e := range r.entries {
		select {
		case <-e.done:
			if e.refs == 0 && e.waiters == 0 && now.Sub(e.idle) >= r.idle {
				delete(r.entries, key)
				r.evicted++
			}
		default:
		}
	}
}
func (r *Registry) Evict(now time.Time) { r.mu.Lock(); defer r.mu.Unlock(); r.evictLocked(now) }

// DrainSurface closes entry to this source before allowing its writer to delete
// persistent scenery. An acquired pin or in-flight load delays cleanup.
func (r *Registry) DrainSurface(surface int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retiring[surface] = true
	drained := true
	for key, e := range r.entries {
		if key.Surface != surface {
			continue
		}
		e.cancel()
		select {
		case <-e.done:
			if e.refs == 0 && e.waiters == 0 {
				delete(r.entries, key)
			} else {
				drained = false
			}
		default:
			drained = false
		}
	}
	return drained
}
func (r *Registry) Count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.entries) }

// All scenery was committed before any pin was returned; eviction/shutdown has
// no unsaved game state. Character checkpointing belongs to later stages.
func (r *Registry) Close(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.cancel()
		go func() { r.wg.Wait(); close(r.finished) }()
	}
	r.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.finished:
		return nil
	}
}
