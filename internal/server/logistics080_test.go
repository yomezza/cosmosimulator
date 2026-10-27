package server

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestLogisticsTransfers080(t *testing.T) {
	f := newFixture033(t)
	one, uid := f.signup("logistics080")
	two, other := f.signup("logother080")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	otherShip := f.integer(`SELECT id FROM ships WHERE user_id=$1`, other)
	f.exec(`UPDATE ships SET cargo='{"iron":40,"agni":10}',fuel=40 WHERE id=$1`, sid)
	from := CargoLocation{Kind: "ship", Ship: sid}
	to := CargoLocation{Kind: "personal", Star: 1, Station: "station"}
	body := map[string]any{"request": "transfer-080-00000001", "mode": "direct", "from": from, "to": to, "ship": sid, "resource": "iron", "quantity": 7}
	f.req(one, "POST", "/api/logistics", body, 200)
	f.req(one, "POST", "/api/logistics", body, 200)
	if f.integer(`SELECT (cargo->>'iron')::int FROM ships WHERE id=$1`, sid) != 33 || f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=1 AND station='station'`, uid) != 7 {
		t.Fatal("retry duplicated inventory transfer")
	}
	body["quantity"] = 8
	f.req(one, "POST", "/api/logistics", body, 409)
	body["request"] = "transfer-080-00000002"
	body["quantity"] = 1
	f.req(two, "POST", "/api/logistics", body, 409)
	alias := to
	alias.Company = 999
	alias.Ship = sid
	body["from"] = alias
	body["to"] = to
	f.req(one, "POST", "/api/logistics", body, 400)
	if f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=1 AND station='station'`, uid) != 7 {
		t.Fatal("alias made resources")
	}
	body["from"] = from
	body["to"] = CargoLocation{Kind: "personal", Star: 5, Station: "station"}
	f.req(one, "POST", "/api/logistics", body, 409)
	body["mode"] = "queue"
	f.req(one, "POST", "/api/logistics", body, 200)
	f.req(one, "POST", "/api/logistics", body, 200)
	if f.integer(`SELECT count(*) FROM automation_tasks WHERE user_id=$1`, uid) != 1 {
		t.Fatal("delivery was queued twice")
	}
	for range 12 {
		advanceTasks070(f, uid)
	}
	if f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=5 AND station='station'`, uid) != 1 {
		t.Fatal("unload task did not deliver")
	}
	body = map[string]any{"request": "transfer-080-00000003", "mode": "queue", "from": CargoLocation{Kind: "personal", Star: 1, Station: "station"}, "to": from, "ship": sid, "resource": "iron", "quantity": 4}
	f.req(one, "POST", "/api/logistics", body, 200)
	for range 12 {
		advanceTasks070(f, uid)
	}
	if f.integer(`SELECT (cargo->>'iron')::int FROM ships WHERE id=$1`, sid) != 36 {
		t.Fatal("load task did not retrieve real cargo")
	}
	// A corporate store requires both a completed facility and current permissions.
	f.exec(`UPDATE users SET credits=2000000 WHERE id=$1`, uid)
	f.req(one, "POST", "/api/companies", map[string]any{"action": "leave"}, 200)
	f.req(one, "POST", "/api/companies", map[string]any{"action": "create", "name": "Logistics Lab", "tag": "LAB", "flag": "◆", "color": "#80bfb1"}, 200)
	cid := f.integer(`SELECT company_id FROM users WHERE id=$1`, uid)
	corp := CargoLocation{Kind: "company", Company: cid, Star: 1, Station: "station"}
	body = map[string]any{"request": "transfer-080-00000004", "mode": "direct", "from": from, "to": corp, "ship": sid, "resource": "iron", "quantity": 5}
	f.req(one, "POST", "/api/logistics", body, 409)
	f.exec(`INSERT INTO company_projects(company_id,name,kind,star,station,status,required) VALUES($1,'Depot','warehouse',1,'station','complete','{}')`, cid)
	f.req(one, "POST", "/api/logistics", body, 200)
	f.exec(`UPDATE users SET company_id=$2 WHERE id=$1`, other, cid)
	body = map[string]any{"request": "transfer-080-00000005", "mode": "direct", "from": corp, "to": CargoLocation{Kind: "ship", Ship: otherShip}, "ship": otherShip, "resource": "iron", "quantity": 2}
	f.req(two, "POST", "/api/logistics", body, 409)
	f.exec(`INSERT INTO company_roles(company_id,name,permissions) VALUES($1,'Keeper','["warehouse"]')`, cid)
	f.exec(`UPDATE users SET company_role=(SELECT id FROM company_roles WHERE company_id=$2 AND name='Keeper') WHERE id=$1`, other, cid)
	f.req(two, "POST", "/api/logistics", body, 200)
	if f.integer(`SELECT (cargo->>'iron')::int FROM company_storage WHERE company_id=$1`, cid) != 3 {
		t.Fatal("corporate stock mismatch")
	}
	f.req(one, "GET", "/api/logistics", nil, 200)
	// The actual reserve under an active mining cycle is included in capacity.
	f.exec(`UPDATE ships SET cargo='{"iron":55}' WHERE id=$1`, otherShip)
	f.exec(`INSERT INTO mining_jobs(user_id,ship_id,star,planet,resource,quantity,finish_tick) VALUES($1,$2,1,101,'iron',4,99999)`, other, otherShip)
	body["request"] = "transfer-080-00000006"
	f.req(two, "POST", "/api/logistics", body, 409)
}
func TestConcurrentLogistics080(t *testing.T) {
	f := newFixture033(t)
	c, uid := f.signup("lastcargo080")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	f.exec(`UPDATE ships SET cargo='{"iron":5}' WHERE id=$1`, sid)
	body := map[string]any{"request": "concurrent-080-000001", "mode": "direct", "from": CargoLocation{Kind: "ship", Ship: sid}, "to": CargoLocation{Kind: "personal", Star: 1, Station: "station"}, "ship": sid, "resource": "iron", "quantity": 5}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); f.req(c, "POST", "/api/logistics", body, 200) }()
	}
	wg.Wait()
	if f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1`, uid) != 5 || f.integer(`SELECT count(*) FROM logistics_operations WHERE user_id=$1`, uid) != 1 {
		t.Fatal("concurrent retry applied twice")
	}
}
func TestAgniShipment080(t *testing.T) {
	f := newFixture033(t)
	c, uid := f.signup("fuelcargo080")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	f.exec(`UPDATE ships SET fuel=0,cargo='{"agni":10}' WHERE id=$1`, sid)
	f.req(c, "POST", "/api/logistics", map[string]any{"request": "fuel-shipment-0800001", "mode": "queue", "from": CargoLocation{Kind: "ship", Ship: sid}, "to": CargoLocation{Kind: "personal", Star: 5, Station: "station"}, "ship": sid, "resource": "agni", "quantity": 10}, 200)
	if err := f.app.advanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT (cargo->>'agni')::int FROM ships WHERE id=$1`, sid) != 10 || f.integer(`SELECT count(*) FROM automation_tasks WHERE user_id=$1 AND status='paused'`, uid) != 1 {
		t.Fatal("shipment was burned as ship fuel")
	}
}
func TestGalaxyDeletion080(t *testing.T) {
	f := newFixture033(t)
	admin, uid := f.signup("delete080")
	player, pid := f.signup("keep080")
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, uid)
	in := GalaxyEdit{Create: true, ID: "delete-test", Name: "Delete Test", Shape: "rings", Seed: 80, Systems: 50, Settings: defaultGalaxySettings()}
	preview := f.req(admin, "POST", "/api/admin/galaxies/preview", in, 200)
	var p struct {
		Preview string `json:"preview"`
	}
	json.Unmarshal(preview.Body.Bytes(), &p)
	in.Preview = p.Preview
	f.req(admin, "POST", "/api/admin/galaxies/apply", in, 200)
	g, _ := f.app.loadUniverse(f.ctx)
	star := coreSystems(g, in.ID)[0]
	f.req(player, "POST", "/api/admin/galaxies/delete-preview", map[string]any{"id": in.ID}, 403)
	f.req(admin, "POST", "/api/admin/galaxies/delete-preview", map[string]any{"id": "edem"}, 400)
	f.exec(`UPDATE ships SET star=$2 WHERE user_id=$1`, pid, star)
	response := f.req(admin, "POST", "/api/admin/galaxies/delete-preview", map[string]any{"id": in.ID}, 200)
	var review struct {
		Preview string `json:"preview"`
		Can     bool   `json:"can_delete"`
	}
	json.Unmarshal(response.Body.Bytes(), &review)
	if review.Can {
		t.Fatal("galaxy with another player's ship was deletable")
	}
	f.req(admin, "POST", "/api/admin/galaxies/delete", map[string]any{"id": in.ID, "preview": review.Preview}, 409)
	f.exec(`UPDATE ships SET star=1 WHERE user_id=$1`, pid)
	f.exec(`INSERT INTO warehouses(user_id,star,station,cargo) VALUES($1,$2,'station','{"iron":0}')`, uid, star)
	response = f.req(admin, "POST", "/api/admin/galaxies/delete-preview", map[string]any{"id": in.ID}, 200)
	json.Unmarshal(response.Body.Bytes(), &review)
	if !review.Can {
		t.Fatal("empty admin galaxy cannot be deleted")
	}
	f.req(admin, "POST", "/api/admin/galaxies/delete", map[string]any{"id": in.ID, "preview": review.Preview}, 200)
	g, _ = f.app.loadUniverse(f.ctx)
	if !g.definition(in.ID).Deleted || len(galaxyStars(g, "edem")) != 3000 || len(g.Stars) != 3050 || g.Stars[star-1].ID != star || !g.Stars[star-1].Deleted || len(g.path(star, star)) != 0 {
		t.Fatal("deletion damaged stable IDs or other galaxies")
	}
	f.req(admin, "GET", "/api/system?id="+fmtInt080(star), nil, 404)
	f.req(admin, "POST", "/api/admin/teleport", map[string]any{"to": star}, 400)
	var schema string
	f.db.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema)
	db, err := openDatabase(f.ctx, f.db.Config().ConnString(), schema)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = reconcileStations(f.ctx, tx, g); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT count(*) FROM stations WHERE star=$1 AND active`, star) != 0 {
		t.Fatal("restart restored archived ports")
	}
}
func fmtInt080(n int) string { b, _ := json.Marshal(n); return string(b) }
