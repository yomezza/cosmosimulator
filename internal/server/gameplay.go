package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"hash/fnv"
	"math"
	"net/http"
	"strconv"
	"time"
)

type Gameplay struct {
	TickSeconds  int       `json:"tick_seconds"`
	DateStep     int       `json:"date_step"`
	Common       int       `json:"quota_common"`
	RareMin      int       `json:"rare_min"`
	RareMax      int       `json:"rare_max"`
	ResetSeconds int       `json:"reset_seconds"`
	Epoch        time.Time `json:"epoch"`
	Generation   int64     `json:"generation"`
}
type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readGameplay(ctx context.Context, q querier) (Gameplay, error) {
	var g Gameplay
	err := q.QueryRow(ctx, `SELECT tick_seconds,date_step,quota_common,rare_min,rare_max,reset_seconds,quota_epoch,generation FROM game_settings WHERE id=1`).Scan(&g.TickSeconds, &g.DateStep, &g.Common, &g.RareMin, &g.RareMax, &g.ResetSeconds, &g.Epoch, &g.Generation)
	return g, err
}
func edemDate(pulses int64) string {
	return fmt.Sprintf("ЭД %04d·%03d·%03d", 1+pulses/360000, 1+(pulses/1000)%360, pulses%1000)
}
func (a *App) gameplay(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if (r.Method == "POST" || r.URL.Path == "/api/admin/gameplay") && u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	if r.Method == "POST" {
		var in struct {
			TickSeconds  int  `json:"tick_seconds"`
			DateStep     int  `json:"date_step"`
			Common       int  `json:"quota_common"`
			RareMin      int  `json:"rare_min"`
			RareMax      int  `json:"rare_max"`
			ResetSeconds int  `json:"reset_seconds"`
			Reset        bool `json:"reset_now"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.TickSeconds < 1 || in.TickSeconds > 3600 || in.DateStep < 1 || in.DateStep > 10000 || in.Common < 3 || in.Common > 1000000 || in.RareMin < 3 || in.RareMax < in.RareMin || in.RareMax > in.Common || in.ResetSeconds < 60 || in.ResetSeconds > 604800 {
			fail(w, 400, "Некорректные настройки игрового процесса")
			return
		}
		tx, err := a.db.Begin(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(r.Context(), `UPDATE game_settings SET tick_seconds=$1,date_step=$2,quota_common=$3,rare_min=$4,rare_max=$5,reset_seconds=$6,generation=generation+CASE WHEN $7 THEN 1 ELSE 0 END,quota_epoch=CASE WHEN $7 THEN now() ELSE quota_epoch END WHERE id=1`, in.TickSeconds, in.DateStep, in.Common, in.RareMin, in.RareMax, in.ResetSeconds, in.Reset); err != nil {
			a.internal(w, err)
			return
		}

		raw, _ := json.Marshal(in)
		if _, err = tx.Exec(r.Context(), `INSERT INTO admin_log(actor,action,data) VALUES($1,'gameplay',$2)`, u.ID, raw); err != nil {
			a.internal(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			a.internal(w, err)
			return
		}
	}
	g, err := readGameplay(r.Context(), a.db)
	if err != nil {
		a.internal(w, err)
		return
	}
	var pulses int64
	var last time.Time
	if err = a.db.QueryRow(r.Context(), `SELECT pulses,last_tick FROM world WHERE id=1`).Scan(&pulses, &last); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]any{"settings": g, "date": edemDate(pulses), "pulses": pulses, "last_tick": last, "server_time": time.Now()})
}

type Quota struct {
	Capacity  int       `json:"capacity"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	Start     time.Time `json:"-"`
}

func quotaCapacity(g Gameplay, star, object int, res string) int {
	if res == "*" {
		return g.Common
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%d:%d:%s", star, object, res)
	return g.RareMin + int(h.Sum32()%uint32(g.RareMax-g.RareMin+1))
}
func ensureQuota(ctx context.Context, tx pgx.Tx, g Gameplay, star, object int, res string, now time.Time, base *Quota) (Quota, error) {
	var q Quota
	start := g.Epoch.Add(time.Duration(math.Floor(now.Sub(g.Epoch).Seconds()/float64(g.ResetSeconds))*float64(g.ResetSeconds)) * time.Second)
	end := start.Add(time.Duration(g.ResetSeconds) * time.Second)
	if base != nil {
		start, end = base.Start, base.ResetAt
	}
	cap := quotaCapacity(g, star, object, res)
	_, err := tx.Exec(ctx, `INSERT INTO resource_quotas(star,object,resource,capacity,window_start,reset_at,generation) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, star, object, res, cap, start, end, g.Generation)
	if err != nil {
		return q, err
	}
	var used int
	var generation int64
	err = tx.QueryRow(ctx, `SELECT capacity,used,window_start,reset_at,generation FROM resource_quotas WHERE star=$1 AND object=$2 AND resource=$3 FOR UPDATE`, star, object, res).Scan(&q.Capacity, &used, &q.Start, &q.ResetAt, &generation)
	if err != nil {
		return q, err
	}
	if generation != g.Generation || !now.Before(q.ResetAt) || base != nil && !q.Start.Equal(base.Start) {
		if generation != g.Generation {
			q.Start = start
			q.ResetAt = end
		} else if base != nil {
			q.Start, q.ResetAt = base.Start, base.ResetAt
		} else {
			elapsed := now.Sub(q.ResetAt)
			q.Start = q.ResetAt.Add(time.Duration(math.Floor(elapsed.Seconds()/float64(g.ResetSeconds))*float64(g.ResetSeconds)) * time.Second)
			q.ResetAt = q.Start.Add(time.Duration(g.ResetSeconds) * time.Second)
		}
		q.Capacity = cap
		used = 0
		if _, err = tx.Exec(ctx, `UPDATE resource_quotas SET capacity=$4,used=0,window_start=$5,reset_at=$6,generation=$7 WHERE star=$1 AND object=$2 AND resource=$3`, star, object, res, q.Capacity, q.Start, q.ResetAt, g.Generation); err != nil {
			return q, err
		}
	}
	q.Remaining = q.Capacity - used
	return q, nil
}
func reserveExtraction(ctx context.Context, tx pgx.Tx, star, object int, res string) (int, time.Time, error) {
	g, err := readGameplay(ctx, tx)
	if err != nil {
		return 0, time.Time{}, err
	}
	now := time.Now()
	common, err := ensureQuota(ctx, tx, g, star, object, "*", now, nil)
	if err != nil {
		return 0, now, err
	}
	remaining := common.Remaining
	if resourceMeta(res).Rarity == "Редкий" {
		rare, err := ensureQuota(ctx, tx, g, star, object, res, now, &common)
		if err != nil {
			return 0, now, err
		}
		remaining = min(remaining, rare.Remaining)
	}
	count := min(3, remaining)
	if count <= 0 {
		return 0, common.Start, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE resource_quotas SET used=used+$4 WHERE star=$1 AND object=$2 AND resource IN ('*',$3)`, star, object, res, count); err != nil {
		return 0, common.Start, err
	}
	return count, common.Start, nil
}
func quotaView(ctx context.Context, tx pgx.Tx, s Star, sites []Site) ([]Site, error) {
	g, err := readGameplay(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range sites {
		if len(sites[i].Resources) == 0 {
			continue
		}
		common, err := ensureQuota(ctx, tx, g, s.ID, sites[i].Planet, "*", now, nil)
		if err != nil {
			return nil, err
		}
		sites[i].SharedQuota = &common
		sites[i].Quotas = map[string]Quota{}
		for _, res := range sites[i].Resources {
			q := common
			if resourceMeta(res).Rarity == "Редкий" {
				q, err = ensureQuota(ctx, tx, g, s.ID, sites[i].Planet, res, now, &common)
				if err != nil {
					return nil, err
				}
				q.Remaining = min(q.Remaining, common.Remaining)
			}
			sites[i].Quotas[res] = q
		}
	}
	return sites, nil
}
func recordEvent(ctx context.Context, tx pgx.Tx, id int64, kind string, details any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO game_events(user_id,kind,data) VALUES($1,$2,$3)`, id, kind, raw)
	return err
}
func (a *App) events(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	admin := r.URL.Path == "/api/admin/log"
	if admin && u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	sql := `SELECT id,kind,data,created_at FROM game_events WHERE user_id=$1 ORDER BY id DESC LIMIT 100`
	args := []any{u.ID}
	if admin {
		sql = `SELECT id,action,data||jsonb_build_object('actor',actor,'actor_name',(SELECT username FROM users WHERE users.id=admin_log.actor)),created_at FROM admin_log ORDER BY id DESC LIMIT 100`
		args = nil
	}
	rows, err := a.db.Query(r.Context(), sql, args...)
	if err != nil {
		a.internal(w, err)
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
			a.internal(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "kind": kind, "data": json.RawMessage(raw), "at": at})
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	reply(w, 200, items)
}
func (a *App) userPages(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
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
		fail(w, 400, "Слишком длинный запрос")
		return
	}
	var total int
	if err := a.db.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE strpos(lower(username),lower($1))>0 OR id::text=$1`, search).Scan(&total); err != nil {
		a.internal(w, err)
		return
	}
	pages := max(1, (total+9)/10)
	page = min(page, pages)
	rows, err := a.db.Query(r.Context(), `SELECT id,username,role,is_bot,credits,level,xp FROM users WHERE strpos(lower(username),lower($1))>0 OR id::text=$1 ORDER BY id LIMIT 10 OFFSET $2`, search, (page-1)*10)
	if err != nil {
		a.internal(w, err)
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
			a.internal(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "username": name, "role": role, "bot": bot, "credits": credits, "level": level, "xp": xp})
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	reply(w, 200, map[string]any{"items": items, "page": page, "pages": pages, "total": total})
}
