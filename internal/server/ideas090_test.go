package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func Test090IdeasPersistenceOwnershipAndReplay(t *testing.T) {
	f := newFixture033(t)
	one, uid := f.signup("idea_author")
	two, other := f.signup("idea_reader")
	admin, adminID := f.signup("idea_admin")
	f.exec(`UPDATE users SET role='admin' WHERE id=$1`, adminID)
	body := map[string]any{"request": strings.Repeat("a", 32), "title": "Новые орбитальные объекты", "description": "Строка первая.\r\n<script>window.attack=true</script>\nO'Reilly", "category": "gameplay"}
	f.req(nil, "GET", "/api/ideas", nil, 401)
	f.req(nil, "POST", "/api/ideas", body, 401)
	f.req(one, "POST", "/api/ideas", body, 201)
	f.req(one, "POST", "/api/ideas", body, 200)
	if f.integer(`SELECT count(*) FROM player_ideas WHERE user_id=$1`, uid) != 1 {
		t.Fatal("retry duplicated a suggestion")
	}
	var description, author string
	if err := f.db.QueryRow(f.ctx, `SELECT description,author_name FROM player_ideas WHERE user_id=$1`, uid).Scan(&description, &author); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(description, "\r") || !strings.Contains(description, "<script>") || author != "idea_author" {
		t.Fatal("stored text or author changed unexpectedly")
	}
	for c, expected := range map[string]int{"mine": 1, "all": 1} {
		cookie := one
		if c == "all" {
			cookie = admin
		}
		w := f.req(cookie, "GET", "/api/ideas?scope="+c, nil, 200)
		var result struct {
			Total int `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		if result.Total != expected {
			t.Fatal(result)
		}
	}
	w := f.req(two, "GET", "/api/ideas", nil, 200)
	if strings.Contains(w.Body.String(), "орбитальные") {
		t.Fatal("another player's suggestions leaked")
	}
	f.req(two, "GET", "/api/ideas?scope=all", nil, 403)
	body["title"] = "Подменённый повтор"
	f.req(one, "POST", "/api/ideas", body, 409)
	body["title"] = "Новые орбитальные объекты"
	f.req(two, "POST", "/api/ideas", body, 201)
	if f.integer(`SELECT count(*) FROM player_ideas WHERE user_id=$1`, other) != 1 {
		t.Fatal("request IDs incorrectly shared between users")
	}
	body["request"] = strings.Repeat("b", 32)
	raw, _ := json.Marshal(body)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/ideas", strings.NewReader(string(raw)))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(one)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 && code != 201 {
			t.Fatal(code)
		}
	}
	if f.integer(`SELECT count(*) FROM player_ideas WHERE user_id=$1`, uid) != 2 {
		t.Fatal("concurrent retry duplicated a suggestion")
	}
	// Emoji count as Unicode characters; only this endpoint accepts up to 16 KiB.
	body["request"] = strings.Repeat("c", 32)
	body["description"] = strings.Repeat("🚀", 3000)
	f.req(one, "POST", "/api/ideas", body, 201)
	body["request"] = strings.Repeat("d", 32)
	body["description"] = strings.Repeat("а", 3001)
	f.req(one, "POST", "/api/ideas", body, 400)
	body["description"] = "Обычное корректное описание идеи"
	body["author"] = "idea_admin"
	f.req(one, "POST", "/api/ideas", body, 400)
	delete(body, "author")
	body["title"] = "x"
	f.req(one, "POST", "/api/ideas", body, 400)
	body["title"] = "Очередная идея"
	body["category"] = "admin"
	f.req(one, "POST", "/api/ideas", body, 400)
	body["category"] = "interface"
	for i := 3; i < 10; i++ {
		body["request"] = fmt.Sprintf("%032x", i)
		f.req(one, "POST", "/api/ideas", body, 201)
	}
	body["request"] = fmt.Sprintf("%032x", 100)
	f.req(one, "POST", "/api/ideas", body, 429)
	// A committed retry is accepted even after the rate limit is reached.
	body["request"] = strings.Repeat("c", 32)
	body["title"] = "Новые орбитальные объекты"
	body["category"] = "gameplay"
	body["description"] = strings.Repeat("🚀", 3000)
	f.req(one, "POST", "/api/ideas", body, 200)
	w = f.req(admin, "GET", "/api/ideas?scope=all&page=1", nil, 200)
	var result struct {
		Total, Page, Pages int
		Items              []map[string]any
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 11 || len(result.Items) != 10 || result.Pages != 2 {
		t.Fatal("pagination", result.Total, len(result.Items), result.Pages)
	}
	w = f.req(admin, "GET", "/api/ideas?scope=all&page=2", nil, 200)
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Items) != 1 {
		t.Fatal("last page")
	}
	var schema string
	f.db.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema)
	db, err := openDatabase(f.ctx, f.db.Config().ConnString(), schema)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if f.integer(`SELECT count(*) FROM player_ideas`) != 11 {
		t.Fatal("restart lost ideas")
	}
}

func Test090PlannerViewsAndResourceCompatibility(t *testing.T) {
	f := newFixture033(t)
	c, uid := f.signup("queue_views")
	_, other := f.signup("queue_other")
	sid := f.integer(`SELECT id FROM ships WHERE user_id=$1`, uid)
	for _, status := range []string{"queued", "running", "paused", "completed", "cancelled"} {
		f.exec(`INSERT INTO automation_tasks(user_id,ship_id,kind,source_star,source_object,dest_star,dest_station,resource,quantity,status) VALUES($1,$2,'haul',1,'station',5,'station','manna',3,$3)`, uid, sid, status)
	}
	f.exec(`INSERT INTO automation_tasks(user_id,ship_id,kind,source_star,source_object,dest_star,dest_station,resource,quantity,status) VALUES($1,(SELECT id FROM ships WHERE user_id=$1),'haul',1,'station',5,'station','agni',3,'queued')`, other)
	for view, expected := range map[string]int{"": 3, "history": 2} {
		path := "/api/planner"
		if view != "" {
			path += "?view=" + view
		}
		w := f.req(c, "GET", path, nil, 200)
		var tasks []map[string]any
		json.Unmarshal(w.Body.Bytes(), &tasks)
		if len(tasks) != expected {
			t.Fatal("task view", view, len(tasks))
		}
		for _, task := range tasks {
			active := task["status"] == "queued" || task["status"] == "running" || task["status"] == "paused"
			if active != (view == "") {
				t.Fatal("wrong task group")
			}
		}
	}
	f.req(c, "GET", "/api/planner?view=forged", nil, 400)
	f.exec(`UPDATE ships SET cargo='{"manna":7,"agni":3,"orichalcum":2}' WHERE id=$1`, sid)
	var schema string
	f.db.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema)
	db, err := openDatabase(f.ctx, f.db.Config().ConnString(), schema)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if f.integer(`SELECT (cargo->>'manna')::int FROM ships WHERE id=$1`, sid) != 7 || f.integer(`SELECT (cargo->>'agni')::int FROM ships WHERE id=$1`, sid) != 3 {
		t.Fatal("resource rename altered cargo IDs")
	}
	seen := map[string]bool{}
	for _, r := range resources {
		meta := resourceMeta(r.ID)
		if meta.Code == "" || seen[meta.Code] {
			t.Fatal("missing/duplicate resource code", r.ID)
		}
		seen[meta.Code] = true
	}
	r, _ := resource("manna")
	if r.Name != "Фосфор" {
		t.Fatal("resource catalog is stale")
	}
	r, _ = resource("agni")
	if r.Name != "Водород" {
		t.Fatal("fuel name is stale")
	}
}
