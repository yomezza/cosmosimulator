package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yomezza/cosmosimulator/internal/universe"
	"github.com/yomezza/cosmosimulator/internal/wallet"
)

func TestWalletExperienceAndResources070(t *testing.T) {
	f := newFixture033(t)
	admin, uid := f.signup("admin070")
	other, oid := f.signup("other070")
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, uid)
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	otherShip := f.integer(`SELECT id FROM ships WHERE user_id=$1`, oid)
	f.req(other, "POST", "/api/admin/resources", map[string]any{}, 403)
	f.cmd(admin, map[string]any{"action": "rename_ship", "ship": sid, "name": "  Северный Ветер  "}, 200)
	if f.integer(`SELECT count(*) FROM ships WHERE id=$1 AND name='Северный Ветер'`, sid) != 1 {
		t.Fatal("rename not persisted")
	}
	f.cmd(other, map[string]any{"action": "rename_ship", "ship": sid, "name": "Stolen"}, 404)
	f.cmd(admin, map[string]any{"action": "rename_ship", "ship": sid, "name": "\n"}, 409)
	f.cmd(admin, map[string]any{"action": "rename_ship", "ship": sid, "name": strings.Repeat("я", 41)}, 409)
	edit := map[string]any{"id": uid, "username": "admin070", "role": "admin", "credits": wallet.MaxCredits, "level": 150, "xp": 15000, "password": ""}
	f.req(admin, "POST", "/api/admin/user", edit, 200)
	f.req(admin, "POST", "/api/profile/upgrade", nil, 200)
	if f.integer(`SELECT level FROM users WHERE id=$1`, uid) != 151 {
		t.Fatal("experience stops at 100")
	}
	edit["credits"] = wallet.MaxCredits + 1
	f.req(admin, "POST", "/api/admin/user", edit, 400)
	f.exec(`UPDATE users SET credits=$2,level=5 WHERE id=$1`, uid, wallet.MaxCredits)
	f.cmd(admin, map[string]any{"action": "buy_ship", "target": "pilgrim"}, 409)
	f.exec(`UPDATE users SET level=6 WHERE id=$1`, uid)
	f.cmd(admin, map[string]any{"action": "buy_ship", "target": "pilgrim"}, 200)
	f.cmd(admin, map[string]any{"action": "buy_ship", "target": "vesper"}, 409)
	f.exec(`UPDATE users SET level=10 WHERE id=$1`, uid)
	f.cmd(admin, map[string]any{"action": "buy_ship", "target": "vesper"}, 200)
	f.exec(`INSERT INTO sell_orders(seller,star,station,resource,quantity,price) VALUES($1,1,'station','iron',50000,1000000)`, oid)
	order := f.integer(`SELECT max(id) FROM sell_orders`)
	f.cmd(admin, map[string]any{"action": "order_buy", "id": order, "quantity": 50000}, 200)
	if f.integer(`SELECT credits FROM users WHERE id=$1`, oid) != 50000001000 {
		t.Fatal("large transfer was truncated")
	}
	f.exec(`UPDATE users SET credits=$2 WHERE id=$1`, oid, wallet.MaxCredits)
	f.exec(`UPDATE sell_orders SET quantity=1 WHERE id=$1`, order)
	before := f.integer(`SELECT credits FROM users WHERE id=$1`, uid)
	f.cmd(admin, map[string]any{"action": "order_buy", "id": order, "quantity": 1}, 409)
	if f.integer(`SELECT credits FROM users WHERE id=$1`, uid) != before || f.integer(`SELECT quantity FROM sell_orders WHERE id=$1`, order) != 1 {
		t.Fatal("overflowing transfer lost money or stock")
	}
	if _, err := f.db.Exec(f.ctx, `UPDATE users SET credits=$2 WHERE id=$1`, uid, wallet.MaxCredits+1); err == nil {
		t.Fatal("ceiling bypassed")
	}
	f.exec(`UPDATE users SET credits=$2 WHERE id=$1`, uid, wallet.MaxCredits)
	f.cmd(admin, map[string]any{"action": "delivery_create", "resource": "iron", "quantity": 1, "amount": wallet.MaxCredits, "collateral": 100, "hours": 1, "to": 5, "station": "station"}, 409)
	if f.integer(`SELECT count(*) FROM deliveries`) != 0 || f.integer(`SELECT credits FROM users WHERE id=$1`, uid) != wallet.MaxCredits {
		t.Fatal("unpayable delivery escrow was accepted")
	}
	grant := map[string]any{"user": oid, "ship": otherShip, "target": "ship", "resource": "iron", "quantity": 10}
	f.req(admin, "POST", "/api/admin/resources", grant, 200)
	grant["ship"] = sid
	f.req(admin, "POST", "/api/admin/resources", grant, 404)
	grant["ship"] = otherShip
	grant["quantity"] = 51
	f.req(admin, "POST", "/api/admin/resources", grant, 409)
	grant["target"] = "warehouse"
	grant["star"] = 1
	grant["station"] = "station"
	grant["quantity"] = 1000000
	f.req(admin, "POST", "/api/admin/resources", grant, 200)
	grant["target"] = "company"
	grant["company"] = 1
	f.req(admin, "POST", "/api/admin/resources", grant, 200)
	if f.integer(`SELECT (cargo->>'iron')::int FROM company_storage WHERE company_id=1 AND star=1 AND station='station'`) != 1000000 || f.integer(`SELECT count(*) FROM admin_log WHERE action='resource_grant'`) != 3 {
		t.Fatal("grant or audit missing")
	}
	if f.integer(`SELECT (cargo->>'iron')::int FROM ships WHERE id=$1`, otherShip) != 10 {
		t.Fatal("failed grant changed cargo")
	}
}

func TestCreateGalaxiesAndPortals070(t *testing.T) {
	f := newFixture033(t)
	admin, uid := f.signup("galaxies070")
	player, _ := f.signup("visitor070")
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, uid)
	for _, shape := range []string{"spiral", "rings", "elliptical", "irregular"} {
		in := universe.GalaxyEdit{Create: true, ID: "new-" + shape, Name: "Test " + shape, Shape: shape, Seed: 7919, Systems: 80, Settings: universe.DefaultGalaxySettings()}
		f.req(player, "POST", "/api/admin/galaxies/preview", in, 403)
		response := f.req(admin, "POST", "/api/admin/galaxies/preview", in, 200)
		var preview struct {
			Preview string `json:"preview"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		g, _ := f.app.World.Load(f.ctx)
		if g.Definition(in.ID) != nil {
			t.Fatal("preview mutated the world")
		}
		in.Preview = preview.Preview
		f.req(admin, "POST", "/api/admin/galaxies/apply", in, 200)
		g, _ = f.app.World.Load(f.ctx)
		if len(universe.GalaxyStars(g, in.ID)) != 80 || g.Definition(in.ID).Shape != shape {
			t.Fatal("new galaxy missing")
		}
		for _, s := range universe.GalaxyStars(g, in.ID) {
			if s.Galaxy != in.ID || s.ID <= 3000 {
				t.Fatal("galaxy membership or IDs corrupted")
			}
		}
		cores := universe.CoreSystems(g, in.ID)
		if len(cores) != 6 {
			t.Fatal("missing black-hole access systems")
		}
		sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
		f.exec(`UPDATE ships SET star=$2 WHERE id=$1`, sid, universe.CoreSystems(g)[0])
		f.cmd(admin, map[string]any{"action": "portal_travel", "target": in.ID}, 200)
		if f.integer(`SELECT star FROM ships WHERE id=$1`, sid) != int64(cores[0]) {
			t.Fatal("wrong portal destination")
		}
		f.cmd(admin, map[string]any{"action": "portal_travel", "target": "edem"}, 200)
	}
}

func plannerCreate070(f *fixture033, c *http.Cookie, body map[string]any) int64 {
	body["action"] = "create"
	f.req(c, "POST", "/api/planner", body, 200)
	return f.integer(`SELECT max(id) FROM automation_tasks`)
}

func advanceTasks070(f *fixture033, uid int64) {
	f.exec(`UPDATE ships SET arrival=CASE WHEN arrival IS NOT NULL THEN now()-interval '1 second' END,local_arrival=CASE WHEN local_arrival IS NOT NULL THEN now()-interval '1 second' END WHERE user_id=$1`, uid)
	f.advance(uid)
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

func TestOfflinePlanner070(t *testing.T) {
	f := newFixture033(t)
	cookie, uid := f.signup("planner070")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	f.exec(`INSERT INTO surveys(user_id,star) VALUES($1,1) ON CONFLICT DO NOTHING`, uid)
	f.exec(`UPDATE ships SET fuel=40,cargo='{"agni":5}',object_id='asteroids',position_mode='approach' WHERE id=$1`, sid)
	g, _ := f.app.World.Load(f.ctx)
	resource := ""
	for _, site := range universe.SystemSites(g.Stars[0]) {
		if site.ID == "asteroids" {
			resource = site.Resources[0]
		}
	}
	task := plannerCreate070(f, cookie, map[string]any{"ship": sid, "kind": "mine", "source_star": 1, "source_object": "asteroids", "dest_star": 1, "dest_station": "station", "resource": resource, "quantity": 7})
	for i := 0; i < 20 && f.integer(`SELECT count(*) FROM automation_tasks WHERE id=$1 AND status='completed'`, task) == 0; i++ {
		advanceTasks070(f, uid)
		if err := f.app.Simulation.AdvanceTick(f.ctx, true); err != nil {
			t.Fatal(err)
		}
	}
	if f.integer(`SELECT progress FROM automation_tasks WHERE id=$1`, task) != 7 || f.integer(`SELECT (cargo->>$2)::int FROM warehouses WHERE user_id=$1 AND star=1 AND station='station'`, uid, resource) != 7 {
		t.Fatal("offline mining did not finish with physical resources")
	}
	if f.integer(`SELECT count(*) FROM notifications WHERE user_id=$1 AND kind='planner'`, uid) != 1 {
		t.Fatal("completion notification missing or duplicated")
	}
	f.exec(`INSERT INTO warehouses(user_id,star,station,cargo) VALUES($1,1,'station','{"iron":75}') ON CONFLICT(user_id,star,station) DO UPDATE SET cargo=EXCLUDED.cargo`, uid)
	haul := plannerCreate070(f, cookie, map[string]any{"ship": sid, "kind": "haul", "source_star": 1, "source_object": "station", "dest_star": 5, "dest_station": "station", "resource": "iron", "quantity": 75})
	for i := 0; i < 30 && f.integer(`SELECT count(*) FROM automation_tasks WHERE id=$1 AND status='completed'`, haul) == 0; i++ {
		advanceTasks070(f, uid)
	}
	if f.integer(`SELECT progress FROM automation_tasks WHERE id=$1`, haul) != 75 || f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=5 AND station='station'`, uid) != 75 {
		t.Fatal("multi-trip warehouse transport failed")
	}
	missing := plannerCreate070(f, cookie, map[string]any{"ship": sid, "kind": "haul", "source_star": 1, "source_object": "station", "dest_star": 5, "dest_station": "station", "resource": "iron", "quantity": 10})
	for range 8 {
		advanceTasks070(f, uid)
	}
	if f.integer(`SELECT count(*) FROM automation_tasks WHERE id=$1 AND status='paused' AND error<>''`, missing) != 1 {
		t.Fatal("missing cargo should pause, not duplicate cargo")
	}
	f.req(cookie, "POST", "/api/planner", map[string]any{"action": "cancel", "id": missing}, 200)
}

func TestPlannerRecovery070(t *testing.T) {
	f := newFixture033(t)
	c, uid := f.signup("recovery070")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	f.exec(`INSERT INTO surveys(user_id,star) VALUES($1,1) ON CONFLICT DO NOTHING`, uid)
	f.exec(`UPDATE ships SET object_id='asteroids',position_mode='approach' WHERE id=$1`, sid)
	g, _ := f.app.World.Load(f.ctx)
	res := ""
	for _, site := range universe.SystemSites(g.Stars[0]) {
		if site.ID == "asteroids" {
			res = site.Resources[0]
		}
	}
	task := plannerCreate070(f, c, map[string]any{"ship": sid, "kind": "mine", "source_star": 1, "source_object": "asteroids", "dest_star": 1, "resource": res, "quantity": 6})
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.req(c, "POST", "/api/expedition", map[string]any{"action": "cancel_mine"}, 409)
	f.req(c, "POST", "/api/planner", map[string]any{"action": "pause", "id": task}, 200)
	f.req(c, "POST", "/api/expedition", map[string]any{"action": "cancel_mine"}, 200)
	if f.integer(`SELECT progress FROM automation_tasks WHERE id=$1`, task) != 0 {
		t.Fatal("cancelled cycle awarded progress")
	}
	f.req(c, "POST", "/api/planner", map[string]any{"action": "resume", "id": task}, 200)
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := f.db.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	db, err := openDatabase(f.ctx, f.db.Config().ConnString(), schema)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if f.integer(`SELECT quantity FROM mining_jobs WHERE automation_id=$1`, task) != 3 {
		t.Fatal("in-progress cycle lost on restart")
	}
	f.req(c, "POST", "/api/planner", map[string]any{"action": "pause", "id": task}, 200)
	if err := f.app.Simulation.AdvanceTick(f.ctx, true); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT progress FROM automation_tasks WHERE id=$1`, task) != 3 {
		t.Fatal("paused cycle did not retain actual extraction receipt")
	}
	f.req(c, "POST", "/api/planner", map[string]any{"action": "resume", "id": task}, 200)
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Simulation.AdvanceTick(f.ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT progress FROM automation_tasks WHERE id=$1 AND status='completed'`, task) != 6 || f.integer(`SELECT (cargo->>$2)::int FROM ships WHERE id=$1`, sid, res) != 6 {
		t.Fatal("restart or cancellation duplicated/lost mining output")
	}
}
