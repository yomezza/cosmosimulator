package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestServiceMetrics230(t *testing.T) {
	f, c, uid, _, _, _ := prepare180(t)
	base, stop := startPlanet150(t, f.db)
	defer stop()
	requireLoad230(t, base, nil, "GET", "/planet/api/v1/metrics", nil, 401)
	requireLoad230(t, base, c, "GET", "/planet/api/v1/metrics", nil, 403)
	f.req(nil, "GET", "/api/admin/metrics", nil, 401)
	f.req(c, "GET", "/api/admin/metrics", nil, 403)
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, uid)
	v := requireLoad230(t, base, c, "GET", "/planet/api/v1/metrics", nil, 200)
	r := f.req(c, http.MethodGet, "/api/admin/metrics", nil, 200)
	var cosmic map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &cosmic); err != nil {
		t.Fatal(err)
	}
	for _, data := range []map[string]any{v, cosmic} {
		p, ok := data["process"].(map[string]any)
		if !ok || p["sql_calls"].(float64) <= 0 {
			t.Fatal("missing bounded aggregate metrics")
		}
		raw, _ := json.Marshal(data)
		for _, secret := range []string{c.Value, "password_hash", "token_hash", "postgres://", "SELECT ", "username", "seed"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("metrics exposed private data:", secret)
			}
		}
	}
	f.exec(`UPDATE users SET role='player' WHERE id=$1`, uid)
	requireLoad230(t, base, c, "GET", "/planet/api/v1/metrics", nil, 403)
}
