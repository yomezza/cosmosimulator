package administration

import (
	"net/http"

	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
)

func (a *Service) adminUsers(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	rows, err := a.DB.Query(r.Context(), `SELECT id,username,role,is_bot,credits,ore,level,xp FROM users ORDER BY id LIMIT 200`)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, credits, ore int64
		var name, role string
		var bot bool
		var level, xp int
		if err = rows.Scan(&id, &name, &role, &bot, &credits, &ore, &level, &xp); err != nil {
			httpkit.Internal(w, err)
			return
		}
		list = append(list, map[string]any{"id": id, "username": name, "role": role, "bot": bot, "credits": credits, "ore": ore, "level": level, "xp": xp})
	}
	if rows.Err() != nil {
		httpkit.Internal(w, rows.Err())
		return
	}
	httpkit.Reply(w, 200, list)
}
