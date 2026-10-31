package telemetry

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLQueries counts client Query/Exec calls, including calls inside transactions.
// It never stores a query, its arguments, a DSN or a context value.
type SQLQueries struct {
	queries  atomic.Uint64
	failures atomic.Uint64
}

func (s *SQLQueries) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	s.queries.Add(1)
	return ctx
}
func (s *SQLQueries) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err != nil {
		s.failures.Add(1)
	}
}

func Process(db *pgxpool.Pool) map[string]any {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := db.Stat()
	v := map[string]any{
		"heap_alloc_bytes": m.HeapAlloc, "heap_inuse_bytes": m.HeapInuse, "go_sys_bytes": m.Sys, "goroutines": runtime.NumGoroutine(), "gc_cycles": m.NumGC,
		"pool": map[string]any{"acquired": s.AcquiredConns(), "total": s.TotalConns(), "max": s.MaxConns(), "acquires": s.AcquireCount(), "acquire_ms": float64(s.AcquireDuration()) / float64(time.Millisecond), "empty_acquires": s.EmptyAcquireCount(), "empty_wait_ms": float64(s.EmptyAcquireWaitTime()) / float64(time.Millisecond), "canceled_acquires": s.CanceledAcquireCount()},
	}
	if q, ok := db.Config().ConnConfig.Tracer.(*SQLQueries); ok {
		v["sql_calls"] = q.queries.Load()
		v["sql_errors"] = q.failures.Load()
	}
	return v
}
