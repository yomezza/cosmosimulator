package integration

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This TCP proxy interrupts ONLY the planetary pool, in an already disposable
// test DB/schema. Cosmic keeps its own direct connection and can still tick.
func databaseWire200(t *testing.T, f *fixture033) (*pgxpool.Pool, func()) {
	t.Helper()
	cfg := f.db.Config().Copy()
	target := net.JoinHostPort(cfg.ConnConfig.Host, strconv.Itoa(int(cfg.ConnConfig.Port)))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	stopped := false
	conns := map[net.Conn]bool{}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			left, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if stopped {
				mu.Unlock()
				left.Close()
				return
			}
			conns[left] = true
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer left.Close()
				right, err := net.DialTimeout("tcp", target, time.Second)
				if err != nil {
					return
				}
				defer right.Close()
				mu.Lock()
				if stopped {
					mu.Unlock()
					return
				}
				conns[right] = true
				mu.Unlock()
				done := make(chan struct{})
				go func() { io.Copy(right, left); right.Close(); close(done) }()
				io.Copy(left, right)
				left.Close()
				<-done
				mu.Lock()
				delete(conns, left)
				delete(conns, right)
				mu.Unlock()
			}()
		}
	}()
	var once sync.Once
	cut := func() {
		once.Do(func() {
			mu.Lock()
			stopped = true
			listener.Close()
			for c := range conns {
				c.Close()
			}
			mu.Unlock()
			workers.Wait()
		})
	}
	t.Cleanup(cut)
	addr := listener.Addr().(*net.TCPAddr)
	cfg.ConnConfig.Host = "127.0.0.1"
	cfg.ConnConfig.Port = uint16(addr.Port)
	cfg.MaxConns = 4
	db, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db, cut
}
func TestPlanetDatabaseRecovery200(t *testing.T) {
	f, c, uid, _, site, ship := prepare180(t)
	db, cut := databaseWire200(t, f)
	base, stop := startPlanet150(t, db)
	ticket := ticket180(t, f, c, op180(t), site, ship)
	conn, w := ws180(t, base, c, map[string]any{"ticket": ticket["ticket"]})
	x, y := int(w["x_q"].(float64)), int(w["y_q"].(float64))
	command190(t, conn, w, 2, "move", map[string]int{"x_q": x + 1024, "y_q": y})
	read190(t, conn, "path.accepted")
	for {
		state := read190(t, conn, "state.delta")
		if state["x_q"].(float64) > float64(x)+700 {
			break
		}
	}
	// Takeover forces one safe checkpoint; subsequent movement stays RAM-only.
	next, w2 := ws180(t, base, c, map[string]any{"resume_visit": w["visit"]})
	conn.CloseNow()
	savedX := f.integer(`SELECT x_q FROM planet_characters WHERE user_id=$1`, uid)
	savedY := f.integer(`SELECT y_q FROM planet_characters WHERE user_id=$1`, uid)
	fence := f.integer(`SELECT fence FROM planet_runtime_leases WHERE id=1`)
	command190(t, next, w2, 2, "move", map[string]int{"x_q": int(savedX) + 2*1024, "y_q": int(savedY)})
	read190(t, next, "path.accepted")
	for {
		state := read190(t, next, "state.delta")
		if state["x_q"].(float64) > float64(savedX)+500 {
			break
		}
	}
	cut()
	failure := read190(t, next, "error")
	if failure["code"] != "runtime_unavailable" && failure["code"] != "save_failed" {
		t.Fatal(failure)
	}
	stop()
	db.Close()
	next.CloseNow()
	if f.integer(`SELECT x_q FROM planet_characters WHERE user_id=$1`, uid) != savedX {
		t.Fatal("outage wrote unconfirmed RAM position")
	}
	if err := f.app.Simulation.AdvanceTick(f.ctx, true); err != nil {
		t.Fatal("cosmic tick failed with planet DB disconnected", err)
	}
	replacement, stopReplacement := startPlanet150(t, f.db)
	defer stopReplacement()
	waitPlanetReady150(t, replacement)
	resumed, result := ws180(t, replacement, c, map[string]any{"resume_visit": w["visit"]})
	defer resumed.CloseNow()
	if result["visit"] != w["visit"] || result["actor"] != w["actor"] || int64(result["x_q"].(float64)) != savedX || int64(result["y_q"].(float64)) != savedY {
		t.Fatal("checkpoint/identity lost after DB outage", result)
	}
	if f.integer(`SELECT fence FROM planet_runtime_leases WHERE id=1`) <= fence {
		t.Fatal("replacement did not fence old runtime")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := db.Ping(ctx); err == nil {
		t.Fatal("broken planetary pool unexpectedly works")
	}
}
