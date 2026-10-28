package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestFleetAllowances110(t *testing.T) {
	for _, c := range []struct{ level, want int }{{1, 2}, {5, 2}, {6, 3}, {10, 3}, {11, 4}, {16, 4}, {17, 6}, {25, 6}, {26, 7}, {35, 7}, {36, 7}, {1000, 7}} {
		if got := fleetLimit(c.level); got != c.want {
			t.Fatalf("level %d: %d != %d", c.level, got, c.want)
		}
	}
}
func TestFleetPublicAndLimits110(t *testing.T) {
	f := newFixture033(t)
	cookie, uid := f.signup("fleet110")
	_, other := f.signup("public110")
	f.exec(`UPDATE users SET credits=600000000000,level=6 WHERE id=$1`, uid)
	var star int
	var station string
	var price int64
	if err := f.db.QueryRow(f.ctx, `SELECT y.star,y.station,y.price FROM shipyard_stock y JOIN stations s ON s.star=y.star AND s.object_id=y.station WHERE y.ship_class='pilgrim' AND y.quantity>0 AND s.active AND s.shipyard LIMIT 1`).Scan(&star, &station, &price); err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE shipyard_stock SET quantity=10 WHERE star=$1 AND station=$2 AND ship_class='pilgrim'`, star, station)
	f.exec(`UPDATE ships SET star=$2,object_id=$3,position_mode='docked' WHERE user_id=$1`, uid, star, station)
	// With one starter, only two concurrent purchases may fit at experience 6.
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := f.reqAny110(cookie, map[string]any{"action": "buy_ship", "target": "pilgrim", "star": star, "station": station, "price": price})
			if r != 200 && r != 409 {
				t.Errorf("purchase status %d", r)
			}
		}()
	}
	wg.Wait()
	if n := f.integer(`SELECT count(*) FROM ships WHERE user_id=$1`, uid); n != 3 {
		t.Fatalf("fleet limit bypassed: %d", n)
	}
	if got := f.integer(`SELECT credits FROM users WHERE id=$1`, uid); got != 600000000000-2*price {
		t.Fatal("wrong debit", got)
	}
	// Lowered experience preserves existing ships but refuses any further purchase.
	f.exec(`UPDATE users SET level=1 WHERE id=$1`, uid)
	f.cmd(cookie, map[string]any{"action": "buy_ship", "target": "pilgrim", "star": star, "station": station, "price": price}, 409)
	f.exec(`UPDATE ships SET cargo='{"iron":23}',fuel=7 WHERE user_id=$1`, other)
	out := f.req(cookie, "GET", "/api/fleet", nil, 200)
	var data struct {
		Ships  []Ship           `json:"ships"`
		Public []map[string]any `json:"public_ships"`
		Limit  int              `json:"ship_limit"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Ships) != 3 || len(data.Public) != 1 || data.Limit != 2 {
		t.Fatal(out.Body.String())
	}
	for _, key := range []string{"cargo", "fuel", "credits", "extractor", "spec"} {
		if _, ok := data.Public[0][key]; ok {
			t.Fatal("private field leaked", key)
		}
	}
	if data.Public[0]["owner_name"] != "public110" {
		t.Fatal("owner missing")
	}
	if !strings.Contains(out.Body.String(), "public_ships") {
		t.Fatal("missing public fleet")
	}
}

func (f *fixture033) reqAny110(c *http.Cookie, body any) int {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/api/workbench", bytes.NewReader(raw))
	r.AddCookie(c)
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w.Code
}
