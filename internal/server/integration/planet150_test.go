package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	planetruntime "github.com/yomezza/cosmosimulator/internal/planet/runtime"
	"github.com/yomezza/cosmosimulator/internal/planetserver"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

func startPlanet150(t *testing.T, db *pgxpool.Pool) (string, func()) {
	t.Helper()
	cfg := planetserver.DefaultConfig()
	cfg.DBTimeout = 200 * time.Millisecond
	cfg.LeaseRenew = time.Second
	cfg.LeaseTTL = 3 * time.Second
	cfg.ShutdownTimeout = time.Second
	a, err := planetserver.New(db, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	listening := make(chan struct{})
	go func() { done <- a.Serve(ctx, listener, func() { close(listening) }) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(3 * time.Second):
				t.Error("planet shutdown timed out")
			}
		})
	}
	t.Cleanup(stop)
	select {
	case <-listening:
	case <-time.After(3 * time.Second):
		t.Fatal("planet startup timed out")
	}
	return "http://" + listener.Addr().String(), stop
}

func planetStatus150(t *testing.T, base, path string, want int, code string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	r, err := client.Get(base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	if r.StatusCode != want || !strings.Contains(string(body), code) {
		t.Fatalf("HTTP %d %s", r.StatusCode, body)
	}
}

func waitPlanetReady150(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		r, err := client.Get(base + "/planet/api/v1/health/ready")
		if err == nil {
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			if r.StatusCode == 200 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("planet did not become ready")
}

func TestPlanetFoundation150(t *testing.T) {
	f := newFixture033(t)
	tick := f.integer(`SELECT tick FROM world`)
	galaxies := f.integer(`SELECT count(*) FROM galaxies`)
	users := f.integer(`SELECT count(*) FROM users`)
	first, stop := startPlanet150(t, f.db)
	planetStatus150(t, first, "/planet/api/v1/health/live", 200, "live")
	planetStatus150(t, first, "/planet/api/v1/health/ready", 200, `"gameplay_ready":false`)
	planetStatus150(t, first, "/planet/ws/v1", 503, "planetary_gameplay_not_implemented")
	req, _ := http.NewRequest("GET", first+"/planet/ws/v1", nil)
	req.Header.Set("Origin", "https://untrusted.example")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("Origin allowed", r.StatusCode)
	}
	second, stopSecond := startPlanet150(t, f.db)
	planetStatus150(t, second, "/planet/api/v1/health/live", 200, "live")
	planetStatus150(t, second, "/planet/api/v1/health/ready", 503, "lease_in_use")
	stop()
	waitPlanetReady150(t, second)
	stopSecond()
	if f.integer(`SELECT tick FROM world`) != tick || f.integer(`SELECT count(*) FROM galaxies`) != galaxies || f.integer(`SELECT count(*) FROM users`) != users {
		t.Fatal("planetary startup changed cosmic state")
	}
	cosmic := httptest.NewServer(f.handler)
	defer cosmic.Close()
	r, err = http.Get(cosmic.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal("cosmic health after planetary stop", r.StatusCode)
	}
	if f.integer(`SELECT count(*) FROM planet_runtime_leases WHERE owner_id IS NOT NULL`) != 0 {
		t.Fatal("lease retained on shutdown")
	}
}

func TestPlanetSchema150(t *testing.T) {
	f := newFixture033(t)
	missing := fmt.Sprintf("planet_missing_%d", time.Now().UnixNano())
	db, _, err := postgres.Open(f.ctx, os.Getenv("TEST_DATABASE_URL"), missing)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	address, stop := startPlanet150(t, db)
	planetStatus150(t, address, "/planet/api/v1/health/ready", 503, "schema_missing")
	stop()
	if f.integer(`SELECT count(*) FROM pg_namespace WHERE nspname=$1`, missing) != 0 {
		t.Fatal("planet created a schema")
	}
	f.exec(`UPDATE planet_schema_state SET version=99 WHERE id=1`)
	address, stop = startPlanet150(t, f.db)
	planetStatus150(t, address, "/planet/api/v1/health/ready", 503, "schema_incompatible")
	f.exec(`UPDATE planet_schema_state SET version=$1 WHERE id=1`, planetruntime.SchemaVersion)
	waitPlanetReady150(t, address)
	f.exec(`DELETE FROM planet_schema_state`)
	planetStatus150(t, address, "/planet/api/v1/health/ready", 503, "schema_missing")
	stop()
}

func TestPlanetFencing150(t *testing.T) {
	f := newFixture033(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	old, _ := planetruntime.NewCoordinator(f.db, 15*time.Second, logger)
	successor, _ := planetruntime.NewCoordinator(f.db, 15*time.Second, logger)
	old.Step(f.ctx)
	previous, reason := old.Current()
	if reason != "" {
		t.Fatal(reason)
	}
	f.exec(`UPDATE planet_runtime_leases SET expires_at=clock_timestamp()-interval '1 second'`)
	if old.Ready(f.ctx) != "lease_lost" {
		t.Fatal("expired lease reported ready")
	}
	successor.Step(f.ctx)
	next, reason := successor.Current()
	if reason != "" || next.Fence <= previous.Fence {
		t.Fatal("successor failed to fence", reason)
	}
	if err := old.Release(f.ctx); err != nil {
		t.Fatal(err)
	}
	if successor.Ready(f.ctx) != "" {
		t.Fatal("old process cleared successor lease")
	}
	old.Step(f.ctx)
	_, reason = old.Current()
	if reason != "lease_in_use" {
		t.Fatal("old process reacquired active lease", reason)
	}
	successor.Release(f.ctx)
}

func TestPlanetRestrictedRole150(t *testing.T) {
	f := newFixture033(t)
	role := fmt.Sprintf("planet_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{role}.Sanitize()
	schema := f.db.Config().ConnConfig.RuntimeParams["search_path"]
	f.exec(`CREATE ROLE ` + quoted + ` NOLOGIN`)
	t.Cleanup(func() {
		f.db.Exec(context.Background(), `DROP OWNED BY `+quoted)
		f.db.Exec(context.Background(), `DROP ROLE `+quoted)
	})
	f.exec(`GRANT USAGE ON SCHEMA ` + schema + ` TO ` + quoted)
	f.exec(`GRANT SELECT ON planet_schema_state,planet_runtime_leases TO ` + quoted)
	f.exec(`GRANT UPDATE(owner_id,fence,expires_at) ON planet_runtime_leases TO ` + quoted)
	cfg := f.db.Config().Copy()
	cfg.ConnConfig.RuntimeParams["role"] = role
	limited, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	var actual string
	if err = limited.QueryRow(f.ctx, `SELECT current_user`).Scan(&actual); err != nil || actual != role {
		t.Fatal("role not applied", err)
	}
	address, stop := startPlanet150(t, limited)
	planetStatus150(t, address, "/planet/api/v1/health/ready", 200, "ready")
	if _, err = limited.Exec(f.ctx, `UPDATE world SET tick=tick+1`); err == nil {
		t.Fatal("planet role can write cosmic time")
	}
	if _, err = limited.Exec(f.ctx, `UPDATE planet_schema_state SET version=2`); err == nil {
		t.Fatal("planet role can migrate")
	}
	if _, err = limited.Exec(f.ctx, `CREATE TABLE planet_forbidden(id int)`); err == nil {
		t.Fatal("planet role can create tables")
	}
	limited.Close()
	planetStatus150(t, address, "/planet/api/v1/health/live", 200, "live")
	planetStatus150(t, address, "/planet/api/v1/health/ready", 503, "database_unavailable")
	stop()
}
