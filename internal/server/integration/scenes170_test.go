package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	planetruntime "github.com/yomezza/cosmosimulator/internal/planet/runtime"
	"github.com/yomezza/cosmosimulator/internal/planet/scene"
	"github.com/yomezza/cosmosimulator/internal/planet/scenestore"
)

func sceneFence170(t *testing.T, db *pgxpool.Pool) (*planetruntime.Coordinator, scenestore.Fence) {
	t.Helper()
	c, err := planetruntime.NewCoordinator(db, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	c.Step(context.Background())
	l, reason := c.Current()
	if reason != "" {
		t.Fatal(reason)
	}
	t.Cleanup(func() { c.Release(context.Background()) })
	return c, scenestore.Fence{Owner: l.Owner, Number: l.Fence}
}
func TestLocalScenes170(t *testing.T) {
	f, _, uid, surface, _ := landingFixture160(t)
	addColony160(f, uid, surface, 1, nil, false)
	addColony160(f, uid, surface, 2, nil, false)
	reconcile160(f)
	_, fence := sceneFence170(t, f.db)
	store := scenestore.New(f.db)
	key := scene.Key{Surface: surface, Sector: 1}
	before := f.integer(`SELECT tick FROM world`)
	balance := f.integer(`SELECT credits FROM users WHERE id=$1`, uid)
	var wg sync.WaitGroup
	results := make(chan *scene.Scene, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := store.Load(f.ctx, key, fence)
			if err != nil {
				failures <- err
			} else {
				results <- s
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if f.integer(`SELECT count(*) FROM planet_local_locations`) != 1 {
		t.Fatal("duplicate persistent scene")
	}
	var packed []byte
	for s := range results {
		b, _, err := scene.Encode(s)
		if err != nil {
			t.Fatal(err)
		}
		if packed != nil && !bytes.Equal(packed, b) {
			t.Fatal("loaders observed different scenery")
		}
		packed = b
	}
	s, err := scenestore.New(f.db).Load(f.ctx, key, fence)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := scene.Encode(s)
	if !bytes.Equal(packed, b) {
		t.Fatal("repository restart rerolled scene")
	}
	// Sector 2 initially has no module. If its territory later becomes separate,
	// the previously reserved candidate accepts its new cosmic-owned platform.
	k2 := scene.Key{Surface: surface, Sector: 2}
	second, err := store.Load(f.ctx, k2, fence)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`DELETE FROM planet_landing_sites WHERE surface=$1 AND sector=1`, surface)
	f.exec(`DELETE FROM planet_colonies WHERE surface=$1 AND sector=1`, surface)
	reconcile160(f)
	if f.integer(`SELECT count(*) FROM planet_landing_sites WHERE surface=$1 AND sector=2`, surface) != 1 {
		t.Fatal("missing new site")
	}
	if err = store.Validate(f.ctx, second, fence); err != nil {
		t.Fatal("late platform did not fit reservation", err)
	}
	f.exec(`UPDATE planet_landing_sites SET x_q=x_q+1024 WHERE surface=$1 AND sector=2`, surface)
	if err = store.Validate(f.ctx, second, fence); !errors.Is(err, scene.ErrLayout) {
		t.Fatal("moved site accepted", err)
	}
	f.exec(`UPDATE planet_landing_sites SET x_q=x_q-1024 WHERE surface=$1 AND sector=2`, surface)
	// Unknown generator/format cannot cause INSERT of an alternate newer scene.
	f.exec(`UPDATE planet_local_locations SET generator_version=99 WHERE surface=$1 AND sector=2`, surface)
	if _, err = store.Load(f.ctx, k2, fence); !errors.Is(err, scene.ErrVersion) {
		t.Fatal("unknown generator silently replaced", err)
	}
	f.exec(`UPDATE planet_local_locations SET generator_version=1,geometry=set_byte(geometry,10,get_byte(geometry,10)#1) WHERE surface=$1 AND sector=2`, surface)
	if _, err = store.Load(f.ctx, k2, fence); !errors.Is(err, scene.ErrCorrupt) {
		t.Fatal("bad checksum silently replaced", err)
	}
	if f.integer(`SELECT count(*) FROM planet_local_locations`) != 2 {
		t.Fatal("invalid snapshot was regenerated")
	}
	if f.integer(`SELECT tick FROM world`) != before || f.integer(`SELECT credits FROM users WHERE id=$1`, uid) != balance {
		t.Fatal("scene generation changed cosmic economy")
	}
}

func TestLocalSceneFencingAndSourceCAS170(t *testing.T) {
	f, _, uid, surface, _ := landingFixture160(t)
	addColony160(f, uid, surface, 1, nil, false)
	reconcile160(f)
	old, fence := sceneFence170(t, f.db)
	store := scenestore.New(f.db)
	key := scene.Key{Surface: surface, Sector: 1}
	// Hold the short commit barrier while the loader reads and computes outside it.
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := store.Load(f.ctx, key, fence); result <- err }()
	deadline := time.Now().Add(5 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if f.integer(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%pg_advisory_xact_lock_shared(784218)%'`) > 0 {
			blocked = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("loader did not reach commit barrier")
	}
	// Simulate a controlled reset starting after the initial source snapshot.
	if _, err = tx.Exec(f.ctx, `UPDATE planet_runtime_controls SET state='retiring' WHERE surface=$1`, surface); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, scene.ErrSource) {
		t.Fatal("source CAS did not reject obsolete generation", err)
	}
	if f.integer(`SELECT count(*) FROM planet_local_locations`) != 0 {
		t.Fatal("obsolete snapshot committed")
	}
	f.exec(`UPDATE planet_runtime_controls SET state='enabled' WHERE surface=$1`, surface)
	s, err := store.Load(f.ctx, key, fence)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE planet_runtime_leases SET expires_at=clock_timestamp()-interval '1 second'`)
	_, next := sceneFence170(t, f.db)
	if next.Number <= fence.Number {
		t.Fatal("no new fence")
	}
	if _, err = store.Load(f.ctx, key, fence); !errors.Is(err, scenestore.ErrFence) {
		t.Fatal("predecessor loaded scene", err)
	}
	if err = store.Validate(f.ctx, s, fence); !errors.Is(err, scenestore.ErrFence) {
		t.Fatal("warm pin skipped fence", err)
	}
	if err = store.PurgeRetired(f.ctx, surface, fence); !errors.Is(err, scenestore.ErrFence) {
		t.Fatal("predecessor can purge", err)
	}
	old.Release(f.ctx)
	if _, err = store.Load(f.ctx, key, next); err != nil {
		t.Fatal("successor cannot resume snapshot", err)
	}
	// Same revision but changed canonical bytes is also refused, preserving evidence.
	f.exec(`UPDATE planet_surfaces SET data=jsonb_set(data,'{heights,0}','999') WHERE id=$1`, surface)
	if _, err = store.Load(f.ctx, key, next); !errors.Is(err, scene.ErrSource) {
		t.Fatal("source checksum ignored", err)
	}
}

func TestLocalSceneRetirement170(t *testing.T) {
	f, c, uid, surface, _ := landingFixture160(t)
	addColony160(f, uid, surface, 1, nil, false)
	reconcile160(f)
	coord, fence := sceneFence170(t, f.db)
	store := scenestore.New(f.db)
	if _, err := store.Load(f.ctx, scene.Key{Surface: surface, Sector: 1}, fence); err != nil {
		t.Fatal(err)
	}
	// A planetary writer cannot purge an enabled source.
	if err := store.PurgeRetired(f.ctx, surface, fence); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT count(*) FROM planet_local_locations`) != 1 {
		t.Fatal("purged enabled source")
	}
	coord.Release(f.ctx)
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, uid)
	planet := int(f.integer(`SELECT planet FROM planet_surfaces WHERE id=$1`, surface))
	info := preview124(f, c, planet)
	w := f.req(c, "POST", "/api/admin/planet/surface", input124(info, "delete", "retire-local170"), 409)
	if !strings.Contains(w.Body.String(), "планетарная служба") || f.integer(`SELECT count(*) FROM planet_local_locations`) != 1 || f.integer(`SELECT count(*) FROM planet_surfaces WHERE id=$1`, surface) != 1 {
		t.Fatal("cosmic deleted planetary data or skipped retirement", w.Body.String())
	}
	_, stop := startPlanet150(t, f.db)
	deadline := time.Now().Add(4 * time.Second)
	for f.integer(`SELECT count(*) FROM planet_local_locations`) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.integer(`SELECT count(*) FROM planet_local_locations`) != 0 {
		t.Fatal("planetary service did not clean retired scenes")
	}
	info = preview124(f, c, planet)
	f.req(c, "POST", "/api/admin/planet/surface", input124(info, "delete", "finish-local170"), 200)
	stop()
	if f.integer(`SELECT count(*) FROM planet_runtime_controls`) != 0 || f.integer(`SELECT count(*) FROM planet_surfaces WHERE id=$1`, surface) != 0 {
		t.Fatal("reset retained source dependencies")
	}
}

func TestLocalSceneRestrictedRole170(t *testing.T) {
	f, _, uid, surface, _ := landingFixture160(t)
	addColony160(f, uid, surface, 1, nil, false)
	reconcile160(f)
	role := fmt.Sprintf("scene_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{role}.Sanitize()
	schema := f.db.Config().ConnConfig.RuntimeParams["search_path"]
	f.exec(`CREATE ROLE ` + quoted + ` NOLOGIN`)
	t.Cleanup(func() {
		f.db.Exec(context.Background(), `DROP OWNED BY `+quoted)
		f.db.Exec(context.Background(), `DROP ROLE `+quoted)
	})
	f.exec(`GRANT USAGE ON SCHEMA ` + schema + ` TO ` + quoted)
	f.exec(`GRANT SELECT ON planet_schema_state,planet_runtime_leases,planet_local_locations,planet_surfaces,planet_surface_controls,planet_runtime_controls,planet_colonies,planet_projects,planet_landing_sites TO ` + quoted)
	f.exec(`GRANT UPDATE(owner_id,fence,expires_at) ON planet_runtime_leases TO ` + quoted)
	f.exec(`GRANT INSERT,DELETE ON planet_local_locations TO ` + quoted)
	cfg := f.db.Config().Copy()
	cfg.ConnConfig.RuntimeParams["role"] = role
	limited, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	coord, fence := sceneFence170(t, limited)
	// Release before pool.Close; cleanup also retries conditionally by owner.
	defer coord.Release(context.Background())
	store := scenestore.New(limited)
	if _, err = store.Load(f.ctx, scene.Key{Surface: surface, Sector: 1}, fence); err != nil {
		t.Fatal("least-privilege scene load", err)
	}
	for _, query := range []string{`UPDATE world SET tick=tick+1`, `UPDATE users SET credits=credits+1`, `UPDATE planet_colonies SET created_at=now()`, `UPDATE planet_landing_sites SET x_q=x_q+1`, `UPDATE planet_runtime_controls SET state='retiring'`, `UPDATE planet_local_locations SET generator_version=2`, `CREATE TABLE forbidden_scene(id int)`} {
		if _, err = limited.Exec(f.ctx, query); err == nil {
			t.Fatal("planetary role can write another owner's data", query)
		}
	}
	f.exec(`UPDATE planet_runtime_controls SET state='retiring' WHERE surface=$1`, surface)
	if err = store.PurgeRetired(f.ctx, surface, fence); err != nil {
		t.Fatal("restricted role cannot clean own retired data", err)
	}
}
