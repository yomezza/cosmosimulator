package administration

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

func (a *Service) AdminChanges(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(1, min(100000, page))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 200 {
		httpkit.Fail(w, 400, "Слишком длинный поиск")
		return
	}
	action := r.URL.Query().Get("action")
	where := ` FROM admin_log l LEFT JOIN users u ON u.id=l.actor WHERE ($1='' OR strpos(lower(COALESCE(u.username,'')||' '||l.action||' '||l.data::text),lower($1))>0) AND ($2='' OR l.action=$2)`
	var total int
	if err := a.DB.QueryRow(r.Context(), `SELECT count(*)`+where, search, action).Scan(&total); err != nil {
		httpkit.Internal(w, err)
		return
	}
	pages := max(1, (total+19)/20)
	page = min(page, pages)
	items, err := postgres.QueryMaps(r.Context(), a.DB, `SELECT l.id,l.actor,COALESCE(u.username,'—') AS actor_name,l.action,l.data,l.created_at`+where+` ORDER BY l.id DESC LIMIT 20 OFFSET $3`, search, action, (page-1)*20)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	httpkit.Reply(w, 200, map[string]any{"items": items, "page": page, "pages": pages, "total": total})
}
