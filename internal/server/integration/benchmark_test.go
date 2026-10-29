package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// The factory must run the same fully wired handlers against a disposable schema
// without touching the caller's balances or leaking virtual accounts.
func TestBenchmarkCompositionIsolation(t *testing.T) {
	f := newFixture033(t)
	cookie, uid := f.signup("benchmark_operator")
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, uid)
	var credits int64
	if err := f.db.QueryRow(f.ctx, `SELECT credits FROM users WHERE id=$1`, uid).Scan(&credits); err != nil {
		t.Fatal(err)
	}
	response := f.req(cookie, "POST", "/api/admin/benchmark", map[string]any{"action": "start", "players": 1, "seconds": 5, "scenario": "mixed"}, 200)
	var runs []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &runs); err != nil || len(runs) != 1 {
		t.Fatalf("benchmark receipt: %v %s", err, response.Body.String())
	}
	id := runs[0].ID
	defer f.req(cookie, "POST", "/api/admin/benchmark", map[string]any{"action": "stop", "id": id}, 200)
	ctx, cancel := context.WithTimeout(f.ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status string
		var requests, failures int64
		var metrics []byte
		err := f.db.QueryRow(ctx, `SELECT status,requests,errors,metrics FROM benchmark_runs WHERE id=$1`, id).Scan(&status, &requests, &failures, &metrics)
		if err != nil {
			t.Fatal(err)
		}
		if status != "running" {
			if status != "completed" || requests == 0 || failures != 0 {
				t.Fatalf("benchmark %s requests=%d errors=%d metrics=%s", status, requests, failures, metrics)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	var after int64
	if err := f.db.QueryRow(f.ctx, `SELECT credits FROM users WHERE id=$1`, uid).Scan(&after); err != nil || after != credits {
		t.Fatalf("source wallet: %d -> %d (%v)", credits, after, err)
	}
	var virtual int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM users WHERE virtual_run IS NOT NULL`).Scan(&virtual); err != nil || virtual != 0 {
		t.Fatalf("virtual users leaked: %d (%v)", virtual, err)
	}
	var schemas int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'benchmark_'||$1::bigint::text||'_%'`, id).Scan(&schemas); err != nil || schemas != 0 {
		t.Fatalf("temporary schema remains: %d (%v)", schemas, err)
	}
}
