package integration

import (
	"strings"
	"testing"
)

func TestIdeaReview144(t *testing.T) {
	f := newFixture033(t)
	player, uid := f.signup("idea_status_player")
	admin, aid := f.signup("idea_status_admin")
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, aid)
	f.req(player, "POST", "/api/ideas", map[string]any{"title": "Test status", "description": "Description for review", "category": "other", "request": strings.Repeat("c", 32)}, 201)
	id := f.integer(`SELECT id FROM player_ideas WHERE user_id=$1`, uid)
	assertStatus := func(want string) {
		t.Helper()
		r := f.req(player, "GET", "/api/ideas?scope=mine", nil, 200)
		if !strings.Contains(r.Body.String(), `"status":"`+want+`"`) {
			t.Fatal(r.Body.String())
		}
	}
	assertStatus("pending")
	body := map[string]any{"id": id, "status": "approved"}
	f.req(nil, "POST", "/api/admin/ideas", body, 401)
	f.req(player, "POST", "/api/admin/ideas", body, 403)
	assertStatus("pending")
	for _, status := range []string{"approved", "rejected", "pending"} {
		body["status"] = status
		f.req(admin, "POST", "/api/admin/ideas", body, 200)
		assertStatus(status)
	}
	body["status"] = "invented"
	f.req(admin, "POST", "/api/admin/ideas", body, 400)
	assertStatus("pending")
	body["status"] = "approved"
	body["id"] = 99999999
	f.req(admin, "POST", "/api/admin/ideas", body, 404)
}
