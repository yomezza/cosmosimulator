package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestStationCommerce040(t *testing.T) {
	f := newFixture033(t)
	cookie, uid := f.signup("commerce040")
	if n := f.integer(`SELECT count(DISTINCT specialization) FROM stations`); n != 8 {
		t.Fatal("missing trade profiles", n)
	}
	for _, condition := range []string{"'iron'=ANY(goods)", "NOT ('iron'=ANY(goods))"} {
		if f.integer(`SELECT count(*) FROM stations WHERE specialization='metals' AND `+condition) == 0 {
			t.Fatal("metallurgical stations have identical iron availability", condition)
		}
	}
	var invalid int
	for _, profile := range tradeProfiles {
		if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM stations WHERE specialization=$1 AND cardinality(goods)>=$2`, profile.ID, len(profile.Goods)).Scan(&invalid); err != nil || invalid != 0 {
			t.Fatal("station offers its entire specialization", profile.ID, invalid, err)
		}
	}
	f.exec(`INSERT INTO stations(star,object_id,name,specialization,goods) VALUES(1,'station-test','Independent test port','metals',ARRAY['iron','carbon'])`)
	f.req(cookie, "POST", "/api/expedition", map[string]any{"action": "buy", "resource": "iron", "quantity": 1}, 200)
	primary := f.integer(`SELECT (stock->>'iron')::int FROM local_markets WHERE star=1 AND station='station'`)
	f.exec(`UPDATE ships SET object_id='station-test' WHERE user_id=$1`, uid)
	f.req(cookie, "POST", "/api/expedition", map[string]any{"action": "buy", "resource": "iron", "quantity": 2}, 200)
	if got := f.integer(`SELECT (stock->>'iron')::int FROM local_markets WHERE star=1 AND station='station'`); got != primary {
		t.Fatal("another port consumed primary stock", got, primary)
	}
	secondary := f.integer(`SELECT (stock->>'iron')::int FROM local_markets WHERE star=1 AND station='station-test'`)
	credits := f.integer(`SELECT credits FROM users WHERE id=$1`, uid)
	f.req(cookie, "POST", "/api/expedition", map[string]any{"action": "buy", "resource": "water", "quantity": 1}, 409)
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "reserve", "star": 1, "station": "station-test", "resource": "water", "quantity": 1}, 409)
	if got := f.integer(`SELECT credits FROM users WHERE id=$1`, uid); got != credits {
		t.Fatal("rejected trade debited wallet")
	}
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "reserve", "star": 1, "station": "station-test", "resource": "iron", "quantity": 3}, 200)
	rid := f.integer(`SELECT max(id) FROM reservations WHERE user_id=$1`, uid)
	f.exec(`UPDATE ships SET object_id='station' WHERE user_id=$1`, uid)
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "collect", "id": rid}, 409)
	f.exec(`UPDATE ships SET object_id='station-test' WHERE user_id=$1`, uid)
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "collect", "id": rid}, 200)
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "reserve", "star": 1, "station": "station-test", "resource": "iron", "quantity": 2}, 200)
	rid = f.integer(`SELECT max(id) FROM reservations WHERE user_id=$1`, uid)
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "cancel", "id": rid}, 200)
	if got := f.integer(`SELECT (stock->>'iron')::int FROM local_markets WHERE star=1 AND station='station-test'`); got != secondary-3 {
		t.Fatal("cancellation restored the wrong inventory", got)
	}
	f.req(cookie, "POST", "/api/market/global", map[string]any{"action": "reserve", "star": 1, "station": "station-test", "resource": "iron", "quantity": 2}, 200)
	f.exec(`UPDATE reservations SET expires_at=now()-interval '1 second' WHERE user_id=$1 AND status='reserved'`, uid)
	if err := f.app.advanceLogistics(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.integer(`SELECT (stock->>'iron')::int FROM local_markets WHERE star=1 AND station='station-test'`); got != secondary-3 {
		t.Fatal("expiry restored the wrong inventory", got)
	}
	f.req(cookie, "GET", "/api/market/global", nil, 200)
}

func TestShipClasses040(t *testing.T) {
	f := newFixture033(t)
	cookie, uid := f.signup("hulls040")
	if n := f.integer(`SELECT count(DISTINCT ship_class) FROM shipyard_stock`); n != int64(len(shipClasses)) {
		t.Fatal("some hulls cannot be bought", n)
	}
	if n := f.integer(`SELECT count(*) FROM (SELECT star,station FROM shipyard_stock GROUP BY star,station HAVING count(*)>=5) y`); n != 0 {
		t.Fatal("a yard offers every model")
	}
	f.exec(`UPDATE users SET credits=1000000 WHERE id=$1`, uid)
	price := f.integer(`SELECT price FROM shipyard_stock WHERE star=1 AND station='station' AND ship_class='wanderer'`)
	stock := f.integer(`SELECT quantity FROM shipyard_stock WHERE star=1 AND station='station' AND ship_class='wanderer'`)
	f.cmd(cookie, map[string]any{"action": "buy_ship", "target": "wanderer", "price": 1}, 409)
	f.cmd(cookie, map[string]any{"action": "buy_ship", "target": "ark"}, 409)
	if f.integer(`SELECT credits FROM users WHERE id=$1`, uid) != 1000000 {
		t.Fatal("failed purchase charged credits")
	}
	f.cmd(cookie, map[string]any{"action": "buy_ship", "target": "wanderer", "price": price}, 200)
	if f.integer(`SELECT credits FROM users WHERE id=$1`, uid) != 1000000-price {
		t.Fatal("wrong ship price")
	}
	if f.integer(`SELECT quantity FROM shipyard_stock WHERE star=1 AND station='station' AND ship_class='wanderer'`) != stock-1 {
		t.Fatal("inventory did not decrease")
	}
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1 AND class='wanderer'`, uid)
	f.cmd(cookie, map[string]any{"action": "select_ship", "ship": sid}, 200)
	f.exec(`UPDATE ships SET fuel=79,cargo='{"agni":1}' WHERE id=$1`, sid)
	f.req(cookie, "POST", "/api/expedition", map[string]any{"action": "refuel"}, 200)
	if n := f.integer(`SELECT fuel FROM ships WHERE id=$1`, sid); n != 80 {
		t.Fatal("refuel ignores hull tank", n)
	}
	f.exec(`UPDATE ships SET cargo='{"iron":35}' WHERE id=$1`, sid)
	f.exec(`INSERT INTO warehouses(user_id,star,station,cargo) VALUES($1,1,'station','{"iron":1}')`, uid)
	f.cmd(cookie, map[string]any{"action": "withdraw", "resource": "iron", "quantity": 1}, 409)
	response := f.req(cookie, "GET", "/api/estimate?to=5", nil, 200)
	var estimate struct {
		Fuel    int `json:"fuel"`
		Seconds int `json:"travel_seconds"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &estimate); err != nil || estimate.Fuel != 2 || estimate.Seconds != 3 {
		t.Fatal("scout route estimate", estimate, err)
	}
	f.req(cookie, "POST", "/api/expedition", map[string]any{"action": "travel", "to": 5}, 200)
	if n := f.integer(`SELECT round(extract(epoch FROM arrival-departure)) FROM ships WHERE id=$1`, sid); n != 3 {
		t.Fatal("scout flight duration", n)
	}
	// Freight capacities must be effective in server operations, not just in the UI.
	f.exec(`UPDATE ships SET class='ark',name='Ковчег',fuel=80,star=1,destination=NULL,arrival=NULL,object_id='station',cargo='{}' WHERE id=$1`, sid)
	f.exec(`UPDATE warehouses SET cargo='{"iron":241}' WHERE user_id=$1`, uid)
	f.cmd(cookie, map[string]any{"action": "withdraw", "resource": "iron", "quantity": 240}, 200)
	f.cmd(cookie, map[string]any{"action": "withdraw", "resource": "iron", "quantity": 1}, 409)
	// Mining hull bonus is applied to actual shared-quota extraction.
	f.exec(`UPDATE ships SET class='forge',fuel=60,object_id='asteroids',position_mode='approach',cargo='{}' WHERE id=$1`, sid)
	f.exec(`INSERT INTO surveys(user_id,star) VALUES($1,1) ON CONFLICT DO NOTHING`, uid)
	g, err := f.app.loadUniverse(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var resourceID string
	for _, site := range systemSites(g.Stars[0]) {
		if site.ID == "asteroids" {
			resourceID = site.Resources[0]
		}
	}
	f.req(cookie, "POST", "/api/expedition", map[string]any{"action": "mine", "planet": 101, "resource": resourceID}, 200)
	if n := f.integer(`SELECT quantity FROM mining_jobs WHERE ship_id=$1`, sid); n != 5 {
		t.Fatal("mining hull bonus missing", n)
	}
}

func TestLastHullPurchase040(t *testing.T) {
	f := newFixture033(t)
	one, uid1 := f.signup("last040one")
	two, uid2 := f.signup("last040two")
	f.exec(`UPDATE users SET credits=100000 WHERE id IN($1,$2)`, uid1, uid2)
	f.exec(`UPDATE shipyard_stock SET quantity=1 WHERE star=1 AND station='station' AND ship_class='wanderer'`)
	var wg sync.WaitGroup
	wg.Add(2)
	for _, cookie := range []*http.Cookie{one, two} {
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"action": "buy_ship", "target": "wanderer"})
			r := httptest.NewRequest("POST", "/api/workbench", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			if w.Code != 200 && w.Code != 409 {
				t.Errorf("purchase: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	if n := f.integer(`SELECT count(*) FROM ships WHERE class='wanderer' AND user_id IN($1,$2)`, uid1, uid2); n != 1 {
		t.Fatal("last hull sold more than once", n)
	}
	if n := f.integer(`SELECT quantity FROM shipyard_stock WHERE star=1 AND station='station' AND ship_class='wanderer'`); n != 0 {
		t.Fatal("invalid last-hull stock", n)
	}
}

func TestLegacyCommerceMigration040(t *testing.T) {
	f := newFixture033(t)
	_, uid := f.signup("migration040")
	f.exec(`UPDATE ships SET fuel=17,cargo='{"iron":7}' WHERE user_id=$1`, uid)
	f.exec(`UPDATE local_markets SET credits=4321,stock='{"iron":10,"orichalcum":9}' WHERE star=1 AND station='station'`)
	f.exec(`UPDATE stations SET ship_stock=0 WHERE star=1 AND object_id='station'`)
	f.exec(`DROP TABLE shipyard_stock;ALTER TABLE ships DROP CONSTRAINT ships_class_fuel;ALTER TABLE ships DROP COLUMN class;ALTER TABLE local_markets DROP CONSTRAINT local_markets_pkey;ALTER TABLE local_markets DROP COLUMN station;ALTER TABLE local_markets ADD PRIMARY KEY(star);ALTER TABLE reservations DROP COLUMN station;ALTER TABLE stations DROP COLUMN goods;ALTER TABLE stations DROP COLUMN yard_seeded`)
	var schema string
	if err := f.db.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		db, err := openDatabase(f.ctx, f.db.Config().ConnString(), schema)
		if err != nil {
			t.Fatal(err)
		}
		db.Close()
		for sql, want := range map[string]int64{fmt.Sprintf(`SELECT fuel FROM ships WHERE user_id=%d`, uid): 17, `SELECT credits FROM local_markets WHERE star=1 AND station='station'`: 4321, `SELECT (stock->>'orichalcum')::int FROM local_markets WHERE star=1 AND station='station'`: 9, `SELECT quantity FROM shipyard_stock WHERE star=1 AND station='station' AND ship_class='pilgrim'`: 0} {
			if got := f.integer(sql); got != want {
				t.Fatal("migration reset existing assets", sql, got, want)
			}
		}
	}
}
