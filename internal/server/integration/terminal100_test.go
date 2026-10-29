package integration

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/yomezza/cosmosimulator/internal/logistics"
)

func TestBatchLogistics100(t *testing.T) {
	f := newFixture033(t)
	c, uid := f.signup("batch100")
	other, oid := f.signup("other100")
	_ = other
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	foreign := f.integer(`SELECT id FROM ships WHERE user_id=$1`, oid)
	f.exec(`UPDATE ships SET cargo='{"iron":5,"carbon":4}' WHERE id=$1`, sid)
	from := logistics.CargoLocation{Kind: "ship", Ship: sid}
	to := logistics.CargoLocation{Kind: "personal", Star: 1, Station: "station"}
	body := map[string]any{"request": "batch100-direct-0001", "mode": "direct", "from": from, "to": to, "ship": sid, "items": []logistics.CargoItem{{Resource: "iron", Quantity: 2}, {Resource: "carbon", Quantity: 3}}}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); f.req(c, "POST", "/api/logistics", body, 200) }()
	}
	wg.Wait()
	if f.integer(`SELECT (cargo->>'iron')::int FROM ships WHERE id=$1`, sid) != 3 || f.integer(`SELECT (cargo->>'carbon')::int FROM ships WHERE id=$1`, sid) != 1 || f.integer(`SELECT (cargo->>'carbon')::int FROM warehouses WHERE user_id=$1 AND star=1`, uid) != 3 {
		t.Fatal("batch duplicated or missed cargo")
	}
	body["items"] = []logistics.CargoItem{{Resource: "iron", Quantity: 1}, {Resource: "carbon", Quantity: 3}}
	f.req(c, "POST", "/api/logistics", body, 409)
	body["request"] = "batch100-rollback-01"
	body["items"] = []logistics.CargoItem{{Resource: "iron", Quantity: 1}, {Resource: "carbon", Quantity: 50}}
	f.req(c, "POST", "/api/logistics", body, 409)
	if f.integer(`SELECT (cargo->>'iron')::int FROM ships WHERE id=$1`, sid) != 3 {
		t.Fatal("partial batch applied")
	}
	body["request"] = "batch100-duplicate01"
	body["items"] = []logistics.CargoItem{{Resource: "iron", Quantity: 1}, {Resource: "iron", Quantity: 1}}
	f.req(c, "POST", "/api/logistics", body, 400)
	body["request"] = "batch100-foreign-001"
	body["items"] = []logistics.CargoItem{{Resource: "iron", Quantity: 1}, {Resource: "carbon", Quantity: 1}}
	body["from"] = logistics.CargoLocation{Kind: "ship", Ship: foreign}
	f.req(c, "POST", "/api/logistics", body, 409)
	f.exec(`UPDATE warehouses SET cargo='{"iron":100,"carbon":100}' WHERE user_id=$1 AND star=1`, uid)
	body["request"] = "batch100-capacity001"
	body["from"] = to
	body["to"] = from
	body["items"] = []logistics.CargoItem{{Resource: "iron", Quantity: 30}, {Resource: "carbon", Quantity: 30}}
	f.req(c, "POST", "/api/logistics", body, 409)
	if f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=1`, uid) != 100 {
		t.Fatal("aggregate capacity overflow partially applied")
	}
	body["request"] = "batch100-queue-00001"
	body["mode"] = "queue"
	body["to"] = logistics.CargoLocation{Kind: "personal", Star: 5, Station: "station"}
	body["items"] = []logistics.CargoItem{{Resource: "iron", Quantity: 2}, {Resource: "carbon", Quantity: 3}}
	f.req(c, "POST", "/api/logistics", body, 200)
	f.req(c, "POST", "/api/logistics", body, 200)
	if f.integer(`SELECT count(*) FROM automation_tasks WHERE user_id=$1`, uid) != 2 || f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=1`, uid) != 100 {
		t.Fatal("remote batch duplicated tasks or teleported cargo")
	}
	f.exec(`INSERT INTO automation_tasks(user_id,ship_id,kind,source_star,source_object,dest_star,dest_station,resource,quantity) SELECT $1,$2,'haul',1,'station',5,'station','iron',1 FROM generate_series(1,17)`, uid, sid)
	body["request"] = "batch100-queue-limit"
	f.req(c, "POST", "/api/logistics", body, 409)
	if f.integer(`SELECT count(*) FROM automation_tasks WHERE user_id=$1`, uid) != 19 {
		t.Fatal("queue limit partially applied batch")
	}
}

func TestPersonalStationLabels100(t *testing.T) {
	f := newFixture033(t)
	a, uid := f.signup("alias100")
	b, _ := f.signup("alias100_other")
	var original string
	if err := f.db.QueryRow(f.ctx, `SELECT name FROM stations WHERE star=1 AND object_id='station'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	f.cmd(a, map[string]any{"action": "station_label", "star": 1, "target": "station", "name": "  Мой порт  ", "note": "Личный склад\r\nКупить топливо"}, 200)
	var label, note, global string
	if err := f.db.QueryRow(f.ctx, `SELECT label,note FROM navigation_notes WHERE user_id=$1 AND star=1 AND object_id='station'`, uid).Scan(&label, &note); err != nil || label != "Мой порт" || note != "Личный склад\nКупить топливо" {
		t.Fatal("private label not saved", label, note, err)
	}
	if err := f.db.QueryRow(f.ctx, `SELECT name FROM stations WHERE star=1 AND object_id='station'`).Scan(&global); err != nil || global != original {
		t.Fatal("global station renamed", err)
	}
	for _, tc := range []struct {
		cookieOwner bool
		want        int
	}{{true, 1}, {false, 0}} {
		cookie := a
		if !tc.cookieOwner {
			cookie = b
		}
		var out struct {
			Notes []map[string]any `json:"notes"`
		}
		if err := json.Unmarshal(f.req(cookie, "GET", "/api/workbench", nil, 200).Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Notes) != tc.want {
			t.Fatal("notes crossed accounts", out.Notes)
		}
	}
	f.cmd(a, map[string]any{"action": "station_label", "star": 1, "target": "planet-1", "name": "Чужой объект"}, 409)
	f.cmd(a, map[string]any{"action": "station_label", "star": 1, "target": "station", "name": strings.Repeat("я", 61)}, 409)
	f.cmd(a, map[string]any{"action": "station_label", "star": 1, "target": "station", "name": strings.Repeat("🚀", 60), "note": strings.Repeat("🛰", 800)}, 200)
	f.cmd(a, map[string]any{"action": "note", "star": 1, "target": "station", "note": "Изменённая заметка", "bookmark": true}, 200)
	if f.integer(`SELECT length(label) FROM navigation_notes WHERE user_id=$1 AND star=1`, uid) != 60 {
		t.Fatal("bookmark update removed alias")
	}
	f.cmd(a, map[string]any{"action": "station_label", "star": 1, "target": "station", "name": "", "note": "Оставить заметку"}, 200)
	if f.integer(`SELECT count(*) FROM navigation_notes WHERE user_id=$1 AND label='' AND note='Оставить заметку' AND bookmark`, uid) != 1 {
		t.Fatal("reset removed note or bookmark")
	}
}

func TestPlannerPriorities100(t *testing.T) {
	f := newFixture033(t)
	c, uid := f.signup("priority100")
	foreign, _ := f.signup("priority100_other")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	f.exec(`INSERT INTO automation_tasks(user_id,ship_id,kind,source_star,source_object,dest_star,dest_station,resource,quantity,source_kind,dest_kind) SELECT $1,$2,'load',5,'station',5,'station','carbon',1,'personal','ship' FROM generate_series(1,3)`, uid, sid)
	first := f.integer(`SELECT min(id) FROM automation_tasks WHERE user_id=$1`, uid)
	last := f.integer(`SELECT max(id) FROM automation_tasks WHERE user_id=$1`, uid)
	f.req(foreign, "POST", "/api/planner", map[string]any{"action": "activate", "id": last}, 404)
	f.req(c, "POST", "/api/planner", map[string]any{"action": "raise", "id": last}, 200)
	if f.integer(`SELECT id FROM automation_tasks WHERE user_id=$1 ORDER BY priority DESC,id OFFSET 1 LIMIT 1`, uid) != last {
		t.Fatal("raise did not change order")
	}
	f.req(c, "POST", "/api/planner", map[string]any{"action": "activate", "id": last}, 200)
	if f.integer(`SELECT id FROM automation_tasks WHERE user_id=$1 ORDER BY priority DESC,id LIMIT 1`, uid) != last {
		t.Fatal("activation did not prioritize task")
	}
	f.exec(`UPDATE automation_tasks SET kind='haul',source_star=1,dest_star=5,resource='iron',quantity=3,carried=3,phase='dest',status='running',dest_kind='personal' WHERE id=$1`, first)
	f.exec(`UPDATE ships SET cargo='{"iron":3}',fuel=30 WHERE id=$1`, sid)
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT count(*) FROM ships WHERE id=$1 AND destination=5 AND cargo='{"iron":3}'`, sid) != 1 || f.integer(`SELECT count(*) FROM automation_tasks WHERE id=$1 AND status='queued'`, last) != 1 {
		t.Fatal("priority switch interrupted entrusted cargo")
	}
	f.exec(`UPDATE ships SET star=5,object_id='station',position_mode='docked',destination=NULL,departure=NULL,arrival=NULL,travel_path='[]' WHERE id=$1`, sid)
	f.exec(`INSERT INTO warehouses(user_id,star,station,cargo) VALUES($1,5,'station','{"carbon":2}') ON CONFLICT(user_id,star,station) DO UPDATE SET cargo='{"carbon":2}'`, uid)
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Planner.AdvanceAutomation(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.integer(`SELECT count(*) FROM automation_tasks WHERE id=$1 AND status='completed'`, last) != 1 || f.integer(`SELECT (cargo->>'iron')::int FROM warehouses WHERE user_id=$1 AND star=5`, uid) != 3 {
		t.Fatal("safe switch did not run prioritized task after unloading")
	}
}
