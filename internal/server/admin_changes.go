package server

import (
	"net/http"
	"strconv"
	"strings"
)

func (a *App) adminChanges(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(1, min(100000, page))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 200 {
		fail(w, 400, "Слишком длинный поиск")
		return
	}
	action := r.URL.Query().Get("action")
	where := ` FROM admin_log l LEFT JOIN users u ON u.id=l.actor WHERE ($1='' OR strpos(lower(COALESCE(u.username,'')||' '||l.action||' '||l.data::text),lower($1))>0) AND ($2='' OR l.action=$2)`
	var total int
	if err := a.db.QueryRow(r.Context(), `SELECT count(*)`+where, search, action).Scan(&total); err != nil {
		a.internal(w, err)
		return
	}
	pages := max(1, (total+19)/20)
	page = min(page, pages)
	items, err := queryMaps(r.Context(), a.db, `SELECT l.id,l.actor,COALESCE(u.username,'—') AS actor_name,l.action,l.data,l.created_at`+where+` ORDER BY l.id DESC LIMIT 20 OFFSET $3`, search, action, (page-1)*20)
	if err != nil {
		a.internal(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	reply(w, 200, map[string]any{"items": items, "page": page, "pages": pages, "total": total})
}
