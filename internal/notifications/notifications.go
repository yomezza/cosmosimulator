package notifications

import (
	"net/http"
	"strings"

	"github.com/yomezza/cosmosimulator/internal/commands"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

func (a *Service) Notifications(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.Method == "POST" {
		var in commands.Command
		if !httpkit.Decode(w, r, &in) {
			return
		}
		if len(in.IDs) > 500 {
			httpkit.Fail(w, 400, "Слишком много записей")
			return
		}
		var err error
		switch in.Action {
		case "delete":
			_, err = a.DB.Exec(ctx, `DELETE FROM notifications WHERE user_id=$1 AND id=ANY($2::bigint[])`, u.ID, in.IDs)
		case "clear":
			_, err = a.DB.Exec(ctx, `DELETE FROM notifications WHERE user_id=$1`, u.ID)
		case "seen":
			_, err = a.DB.Exec(ctx, `UPDATE notifications SET seen=TRUE WHERE user_id=$1 AND (cardinality($2::bigint[])=0 OR id=ANY($2::bigint[]))`, u.ID, in.IDs)
		case "add":
			if len(in.Message) > 500 || !strings.Contains("|success|error|warning|", "|"+in.Level+"|") {
				httpkit.Fail(w, 400, "Некорректное уведомление")
				return
			}
			// Older clients persisted these transient acknowledgements on every click.
			switch strings.TrimSpace(in.Message) {
			case "Действие выполнено", "Тик выполнен", "Action completed", "Tick advanced":
				break
			default:
				_, err = a.DB.Exec(ctx, `INSERT INTO notifications(user_id,kind,level,message) VALUES($1,'message',$2,$3)`, u.ID, in.Level, in.Message)
			}
		default:
			httpkit.Fail(w, 400, "Неизвестное действие")
			return
		}
		if err != nil {
			httpkit.Internal(w, err)
			return
		}
	}
	items, err := postgres.QueryMaps(ctx, a.DB, `SELECT * FROM notifications WHERE user_id=$1 ORDER BY id DESC LIMIT 500`, u.ID)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	var count, unread int
	if err = a.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE NOT seen) FROM notifications WHERE user_id=$1`, u.ID).Scan(&count, &unread); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	httpkit.Reply(w, 200, map[string]any{"items": items, "count": count, "unread": unread})
}
