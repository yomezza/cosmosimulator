package administration

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/yomezza/cosmosimulator/internal/gameplay"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
)

func (a *Service) Gameplay(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if (r.Method == "POST" || r.URL.Path == "/api/admin/gameplay") && u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	if r.Method == "POST" {
		var in struct {
			TickSeconds      int    `json:"tick_seconds"`
			DateStep         int    `json:"date_step"`
			Common           int    `json:"quota_common"`
			RareMin          int    `json:"rare_min"`
			RareMax          int    `json:"rare_max"`
			ResetSeconds     int    `json:"reset_seconds"`
			Reset            bool   `json:"reset_now"`
			ScanSeconds      *int   `json:"scan_seconds"`
			CompanyCost      *int64 `json:"company_cost"`
			RescueCost       *int   `json:"rescue_cost"`
			ReservationHours *int   `json:"reservation_hours"`
		}
		if !httpkit.Decode(w, r, &in) {
			return
		}
		if in.TickSeconds < 1 || in.TickSeconds > 3600 || in.DateStep < 1 || in.DateStep > 10000 || in.Common < 3 || in.Common > 1000000 || in.RareMin < 3 || in.RareMax < in.RareMin || in.RareMax > in.Common || in.ResetSeconds < 60 || in.ResetSeconds > 604800 {
			httpkit.Fail(w, 400, "Некорректные настройки игрового процесса")
			return
		}
		if (in.ScanSeconds != nil && (*in.ScanSeconds < 1 || *in.ScanSeconds > 3600)) || (in.CompanyCost != nil && (*in.CompanyCost < 0 || *in.CompanyCost > 1000000000)) || (in.RescueCost != nil && (*in.RescueCost < 0 || *in.RescueCost > 1000000)) || (in.ReservationHours != nil && (*in.ReservationHours < 1 || *in.ReservationHours > 720)) {
			httpkit.Fail(w, 400, "Некорректные настройки игрового процесса")
			return
		}
		tx, err := a.DB.Begin(r.Context())
		if err != nil {
			httpkit.Internal(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(r.Context(), `UPDATE game_settings SET tick_seconds=$1,date_step=$2,quota_common=$3,rare_min=$4,rare_max=$5,reset_seconds=$6,generation=generation+CASE WHEN $7 THEN 1 ELSE 0 END,quota_epoch=CASE WHEN $7 THEN now() ELSE quota_epoch END WHERE id=1`, in.TickSeconds, in.DateStep, in.Common, in.RareMin, in.RareMax, in.ResetSeconds, in.Reset); err != nil {
			httpkit.Internal(w, err)
			return
		}

		if _, err = tx.Exec(r.Context(), `UPDATE game_settings SET scan_seconds=COALESCE($1,scan_seconds),company_cost=COALESCE($2,company_cost),rescue_cost=COALESCE($3,rescue_cost),reservation_hours=COALESCE($4,reservation_hours) WHERE id=1`, in.ScanSeconds, in.CompanyCost, in.RescueCost, in.ReservationHours); err != nil {
			httpkit.Internal(w, err)
			return
		}
		raw, _ := json.Marshal(in)
		if _, err = tx.Exec(r.Context(), `INSERT INTO admin_log(actor,action,data) VALUES($1,'gameplay',$2)`, u.ID, raw); err != nil {
			httpkit.Internal(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			httpkit.Internal(w, err)
			return
		}
	}
	g, err := gameplay.ReadGameplay(r.Context(), a.DB)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	var pulses int64
	var last time.Time
	if err = a.DB.QueryRow(r.Context(), `SELECT pulses,last_tick FROM world WHERE id=1`).Scan(&pulses, &last); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]any{"settings": g, "date": gameplay.EdemDate(pulses), "pulses": pulses, "last_tick": last, "server_time": time.Now()})
}

func (a *Service) Events(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	admin := r.URL.Path == "/api/admin/log"
	if admin && u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	sql := `SELECT id,kind,data,created_at FROM game_events WHERE user_id=$1 ORDER BY id DESC LIMIT 100`
	args := []any{u.ID}
	if admin {
		sql = `SELECT id,action,data||jsonb_build_object('actor',actor,'actor_name',(SELECT username FROM users WHERE users.id=admin_log.actor)),created_at FROM admin_log ORDER BY id DESC LIMIT 100`
		args = nil
	}
	rows, err := a.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var kind string
		var raw []byte
		var at time.Time
		if err = rows.Scan(&id, &kind, &raw, &at); err != nil {
			httpkit.Internal(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "kind": kind, "data": json.RawMessage(raw), "at": at})
	}
	if rows.Err() != nil {
		httpkit.Internal(w, rows.Err())
		return
	}
	httpkit.Reply(w, 200, items)
}

func (a *Service) UserPages(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	search := r.URL.Query().Get("q")
	if len(search) > 100 {
		httpkit.Fail(w, 400, "Слишком длинный запрос")
		return
	}
	var total int
	if err := a.DB.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE strpos(lower(username),lower($1))>0 OR id::text=$1`, search).Scan(&total); err != nil {
		httpkit.Internal(w, err)
		return
	}
	pages := max(1, (total+9)/10)
	page = min(page, pages)
	rows, err := a.DB.Query(r.Context(), `SELECT id,username,role,is_bot,credits,level,xp FROM users WHERE strpos(lower(username),lower($1))>0 OR id::text=$1 ORDER BY id LIMIT 10 OFFSET $2`, search, (page-1)*10)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, credits int64
		var name, role string
		var bot bool
		var level, xp int
		if err = rows.Scan(&id, &name, &role, &bot, &credits, &level, &xp); err != nil {
			httpkit.Internal(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "username": name, "role": role, "bot": bot, "credits": credits, "level": level, "xp": xp})
	}
	if rows.Err() != nil {
		httpkit.Internal(w, rows.Err())
		return
	}
	httpkit.Reply(w, 200, map[string]any{"items": items, "page": page, "pages": pages, "total": total})
}
