package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Command struct {
	Action      string            `json:"action"`
	ID          int64             `json:"id"`
	Ship        int64             `json:"ship"`
	To          int               `json:"to"`
	Star        int               `json:"star"`
	Station     string            `json:"station"`
	Target      string            `json:"target"`
	Resource    string            `json:"resource"`
	Quantity    int               `json:"quantity"`
	Amount      int64             `json:"amount"`
	Price       int64             `json:"price"`
	Region      int               `json:"region"`
	Name        string            `json:"name"`
	Tag         string            `json:"tag"`
	Color       string            `json:"color"`
	Flag        string            `json:"flag"`
	Note        string            `json:"note"`
	Bookmark    bool              `json:"bookmark"`
	Waypoints   []int             `json:"waypoints"`
	Permissions []string          `json:"permissions"`
	IDs         []int64           `json:"ids"`
	Level       string            `json:"level"`
	Message     string            `json:"message"`
	Hours       int               `json:"hours"`
	Collateral  int64             `json:"collateral"`
	FlagDesign  map[string]string `json:"flag_design"`
}

func (a *App) routes033(m *http.ServeMux) {
	m.HandleFunc("GET /api/estimate", a.estimate)
	m.HandleFunc("GET /api/workbench", a.workbench)
	m.HandleFunc("POST /api/workbench", a.command033)
	m.HandleFunc("GET /api/notifications", a.notifications)
	m.HandleFunc("POST /api/notifications", a.notifications)
	m.HandleFunc("GET /api/admin/benchmark", a.benchmark)
	m.HandleFunc("POST /api/admin/benchmark", a.benchmark)
}
func coreSystems(g *Galaxy) []int {
	list := append([]Star(nil), g.Stars...)
	sort.Slice(list, func(i, j int) bool { return math.Hypot(list[i].X, list[i].Y) < math.Hypot(list[j].X, list[j].Y) })
	out := []int{}
	for _, s := range list {
		if s.ID > 1000 {
			continue
		}
		out = append(out, s.ID)
		if len(out) == 6 {
			break
		}
	}
	return out
}
func ensureStations(ctx context.Context, tx pgx.Tx, g *Galaxy) error {
	var founders []int64
	rows, err := tx.Query(ctx, `SELECT id FROM companies WHERE founder_key IN('founder-01','founder-02') ORDER BY founder_key`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		founders = append(founders, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	core := map[int]bool{}
	for _, id := range coreSystems(g) {
		core[id] = true
	}
	list := []map[string]any{}
	for _, star := range g.Stars {
		sites := systemSites(star)
		if core[star.ID] && !hasStation(star.ID) {
			sites = append(sites, Site{ID: "station", Name: star.Name + " · CORE", Kind: "Орбитальная станция", Orbit: 1})
		}
		for _, s := range sites {
			if s.Kind != "Орбитальная станция" {
				continue
			}
			spec := stationProfile(star.ID, s.ID)
			var founder *int64
			if len(founders) > 0 {
				x := founders[star.Region&1]
				founder = &x
			}
			yard := star.ID == 1 || star.ID == 5 || star.ID%17 == 0
			list = append(list, map[string]any{"star": star.ID, "object_id": s.ID, "name": s.Name, "specialization": spec, "founder": founder, "shipyard": yard, "ship_stock": 2})
		}
	}
	raw, _ := json.Marshal(list)
	_, err = tx.Exec(ctx, `INSERT INTO stations(star,object_id,name,specialization,founder,shipyard,ship_stock) SELECT * FROM jsonb_to_recordset($1) AS x(star int,object_id text,name text,specialization text,founder bigint,shipyard bool,ship_stock int) ON CONFLICT DO NOTHING`, raw)
	if err != nil {
		return err
	}
	return seedCommerce(ctx, tx)
}
func queryMaps(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToMap)
}
func reason(ctx context.Context, tx pgx.Tx, text string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('cosmos.reason',$1,true)`, text)
	return err
}
func (a *App) workbench(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	g, err := a.loadUniverse(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	// Settling a player is transactional and also works without the background loop.
	if err = a.advancePlayer(ctx, u.ID, g); err != nil {
		a.internal(w, err)
		return
	}
	if err = a.seedStations(ctx, g); err != nil {
		a.internal(w, err)
		return
	}
	if _, err = a.db.Exec(ctx, `UPDATE users SET last_active=now() WHERE id=$1`, u.ID); err != nil {
		a.internal(w, err)
		return
	}
	var company *int64
	if err = a.db.QueryRow(ctx, `SELECT company_id FROM users WHERE id=$1`, u.ID).Scan(&company); err != nil {
		a.internal(w, err)
		return
	}
	out := map[string]any{"research": researchView(ctx, a.db, u.ID), "core_systems": coreSystems(g), "server_name": "Иггдрасиль", "ship_classes": shipClasses, "trade_profiles": tradeProfiles, "extractor_price": 250, "reservation_policy": "full_refund", "time": time.Now()}
	queries := []struct {
		key, sql string
		args     []any
	}{
		{"mining", `SELECT j.*,s.name AS ship_name FROM mining_jobs j JOIN ships s ON s.id=j.ship_id WHERE j.user_id=$1 ORDER BY j.ship_id`, []any{u.ID}},
		{"shipyards", `SELECT y.*,s.name AS station_name,s.specialization FROM shipyard_stock y JOIN stations s ON s.star=y.star AND s.object_id=y.station WHERE s.shipyard ORDER BY y.star,y.station,y.ship_class`, nil},
		{"known", `SELECT star,received_at,source FROM surveys WHERE user_id=$1 ORDER BY star`, []any{u.ID}},
		{"notes", `SELECT star,object_id,note,bookmark FROM navigation_notes WHERE user_id=$1`, []any{u.ID}},
		{"stations", `SELECT s.*,c.name AS founder_name,c.color AS founder_color,COALESCE(r.points,0) AS reputation,CASE WHEN c.founder_key='founder-02' AND COALESCE(r.points,0)<20 THEN 2 ELSE 0 END AS fee_percent FROM stations s LEFT JOIN companies c ON c.id=s.founder LEFT JOIN reputation r ON r.company_id=s.founder AND r.user_id=$1 ORDER BY s.star,s.object_id`, []any{u.ID}},
		{"data_offers", `SELECT o.*,u.username AS seller_name,(SELECT count(*) FROM unnest(o.systems) s WHERE NOT EXISTS(SELECT 1 FROM surveys v WHERE v.user_id=$1 AND v.star=s)) AS new_systems FROM survey_offers o JOIN users u ON u.id=o.seller ORDER BY o.id DESC LIMIT 500`, []any{u.ID}},
		{"warehouses", `SELECT * FROM warehouses WHERE user_id=$1 ORDER BY star,station`, []any{u.ID}},
		{"orders", `SELECT o.*,u.username AS seller_name FROM sell_orders o JOIN users u ON u.id=o.seller WHERE quantity>0 ORDER BY id DESC LIMIT 500`, nil},
		{"deliveries", `SELECT d.*,u.username AS owner_name,c.username AS carrier_name FROM deliveries d JOIN users u ON u.id=d.owner LEFT JOIN users c ON c.id=d.carrier ORDER BY id DESC LIMIT 200`, nil},
		{"ledger", `SELECT id,amount,balance,reason,created_at FROM financial_ledger WHERE user_id=$1 ORDER BY id DESC LIMIT 100`, []any{u.ID}},
		{"members", `SELECT u.id,u.username,u.company_role,COALESCE(r.name,CASE WHEN c.owner_id=u.id THEN 'Владелец' ELSE 'Участник' END) AS role_name FROM users u JOIN companies c ON c.id=u.company_id LEFT JOIN company_roles r ON r.id=u.company_role WHERE u.company_id=$1 ORDER BY u.id`, []any{company}},
		{"roles", `SELECT * FROM company_roles WHERE company_id=$1 ORDER BY id`, []any{company}},
		{"requests", `SELECT r.*,u.username FROM company_requests r JOIN users u ON u.id=r.user_id WHERE (r.company_id=$1 OR r.user_id=$2) ORDER BY r.id DESC`, []any{company, u.ID}},
		{"company_finance", `SELECT f.*,u.username FROM company_finance f LEFT JOIN users u ON u.id=f.actor WHERE f.company_id=$1 ORDER BY f.id DESC LIMIT 100`, []any{company}},
		{"company_storage", `SELECT * FROM company_storage WHERE company_id=$1 ORDER BY star,station`, []any{company}},
		{"projects", `SELECT * FROM company_projects WHERE company_id=$1 ORDER BY id`, []any{company}},
		{"project_contributions", `SELECT x.*,u.username FROM project_contributions x JOIN company_projects p ON p.id=x.project_id LEFT JOIN users u ON u.id=x.user_id WHERE p.company_id=$1 ORDER BY x.id DESC LIMIT 100`, []any{company}},
		{"portal", `SELECT * FROM portal`, nil},
		{"reputation", `SELECT r.*,c.name FROM reputation r JOIN companies c ON c.id=r.company_id WHERE user_id=$1`, []any{u.ID}},
		{"population", `SELECT count(*) FILTER(WHERE NOT is_bot AND virtual_run IS NULL AND last_active>now()-interval '5 minutes') AS players,count(*) FILTER(WHERE is_bot AND virtual_run IS NULL) AS bots FROM users`, nil},
	}
	for _, q := range queries {
		v, e := queryMaps(ctx, a.db, q.sql, q.args...)
		if e != nil {
			a.internal(w, e)
			return
		}
		if v == nil {
			v = []map[string]any{}
		}
		out[q.key] = v
	}
	if company != nil {
		var credits int64
		var owner *int64
		if err = a.db.QueryRow(ctx, `SELECT credits,owner_id FROM companies WHERE id=$1`, *company).Scan(&credits, &owner); err != nil {
			a.internal(w, err)
			return
		}
		_, canFinance, accessErr := companyAccess(ctx, a.db, u.ID, "finance")
		if accessErr != nil {
			a.internal(w, accessErr)
			return
		}
		if canFinance {
			out["company_balance"] = credits
		} else {
			out["company_balance"] = nil
			out["company_finance"] = []map[string]any{}
		}
		out["company_owner"] = owner
	}
	reply(w, 200, out)
}
func (a *App) command033(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	var in Command
	if !decode(w, r, &in) {
		return
	}
	g, err := a.loadUniverse(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	ctx := r.Context()
	if err = a.seedStations(ctx, g); err != nil {
		a.internal(w, err)
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, `SELECT credits FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&u.Credits); err != nil {
		a.internal(w, err)
		return
	}
	s, err := ensureShip(ctx, tx, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if in.Ship > 0 {
		var owner int64
		if err = tx.QueryRow(ctx, `SELECT user_id FROM ships WHERE id=$1`, in.Ship).Scan(&owner); err != nil || owner != u.ID {
			fail(w, 404, "Корабль не найден")
			return
		}
		s, err = readShip(ctx, tx, in.Ship)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	if err = reason(ctx, tx, in.Action); err != nil {
		a.internal(w, err)
		return
	}
	handled, err := a.navigationCommand(ctx, tx, u, &s, g, in)
	if !handled {
		handled, err = a.economyCommand(ctx, tx, u, &s, g, in)
	}
	if !handled {
		handled, err = a.corporationCommand(ctx, tx, u, &s, g, in)
	}
	if !handled {
		fail(w, 400, "Неизвестное действие")
		return
	}
	if err != nil {
		var pgerr interface{ SQLState() string }
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Объект не найден")
		} else if errors.As(err, &pgerr) {
			if pgerr.SQLState() == "23505" {
				fail(w, 409, "Такое название уже используется")
			} else {
				a.internal(w, err)
			}
		} else {
			fail(w, 409, err.Error())
		}
		return
	}
	if err = recordEvent(ctx, tx, u.ID, in.Action, map[string]any{"ship": s.ID, "star": s.Star, "resource": in.Resource, "quantity": in.Quantity, "amount": in.Amount}); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	if in.Action == "portal_contribute" {
		a.universeMu.Lock()
		a.universe = nil
		a.universeMu.Unlock()
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func idle(ctx context.Context, tx pgx.Tx, s Ship) error {
	if s.Destination != nil || s.LocalTarget != nil {
		return errors.New("Корабль в пути")
	}
	var busy bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mining_jobs WHERE ship_id=$1) OR EXISTS(SELECT 1 FROM research_jobs WHERE ship_id=$1 AND NOT paused) OR EXISTS(SELECT 1 FROM deliveries WHERE ship_id=$1 AND status='carrying')`, s.ID).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return errors.New("Сначала завершите или приостановите текущее задание")
	}
	return nil
}
func (a *App) navigationCommand(ctx context.Context, tx pgx.Tx, u User, s *Ship, g *Galaxy, in Command) (bool, error) {
	switch in.Action {
	case "select_ship":
		_, err := tx.Exec(ctx, `UPDATE ships SET active=FALSE WHERE user_id=$1 AND active`, u.ID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE ships SET active=TRUE WHERE id=$1 AND user_id=$2`, s.ID, u.ID)
		}
		return true, err
	case "note":
		if in.Star < 1 || in.Star > len(g.Stars) || len(in.Note) > 2000 || len(in.Target) > 80 {
			return true, errors.New("Некорректная заметка")
		}
		_, err := tx.Exec(ctx, `INSERT INTO navigation_notes(user_id,star,object_id,note,bookmark) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,star,object_id) DO UPDATE SET note=EXCLUDED.note,bookmark=EXCLUDED.bookmark`, u.ID, in.Star, in.Target, in.Note, in.Bookmark)
		return true, err
	case "pause_research":
		tag, err := tx.Exec(ctx, `UPDATE research_jobs SET paused=TRUE WHERE user_id=$1 AND ship_id=$2`, u.ID, s.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return true, errors.New("Исследование не найдено")
		}
		return true, err
	case "resume_research":
		var star int
		if err := tx.QueryRow(ctx, `SELECT star FROM research_jobs WHERE ship_id=$1`, s.ID).Scan(&star); err != nil {
			return true, err
		}
		if s.Destination != nil || s.Star != star {
			return true, errors.New("Вернитесь в систему исследования")
		}
		return true, startResearch(ctx, tx, u.ID, s, g.Stars[star-1], g)
	case "explore", "route":
		if err := idle(ctx, tx, *s); err != nil {
			return true, err
		}
		points := in.Waypoints
		if in.Action == "explore" {
			points = []int{in.To}
		}
		if len(points) == 0 || len(points) > 20 {
			return true, errors.New("Укажите от 1 до 20 точек маршрута")
		}
		total := 0
		from := s.Star
		for _, to := range points {
			if to == from {
				continue
			}
			path := g.path(from, to)
			if len(path) < 2 {
				return true, errors.New("Нет допустимого маршрута")
			}
			total += s.hull().JumpFuel * (len(path) - 1)
			from = to
		}
		if total > s.Fuel {
			return true, errors.New("Недостаточно топлива")
		}
		clean := []int{}
		prev := s.Star
		for _, to := range points {
			if to != prev {
				clean = append(clean, to)
				prev = to
			}
		}
		raw, _ := json.Marshal(clean)
		if _, err := tx.Exec(ctx, `UPDATE ships SET waypoints=$2,reserved_fuel=$3 WHERE id=$1`, s.ID, raw, total); err != nil {
			return true, err
		}
		if in.Action == "explore" {
			if in.To < 1 || in.To > len(g.Stars) {
				return true, errors.New("Некорректная система")
			}
			s.Arrival = nil
			if total > 0 {
				at := time.Now().Add(time.Duration(total/2*5) * time.Second)
				s.Arrival = &at
			}
			return true, startResearch(ctx, tx, u.ID, s, g.Stars[in.To-1], g)
		}
		return true, nil
	case "cancel_route":
		_, err := tx.Exec(ctx, `UPDATE ships SET waypoints='[]',reserved_fuel=0 WHERE id=$1`, s.ID)
		return true, err
	case "rescue":
		if err := idle(ctx, tx, *s); err != nil {
			return true, err
		}
		settings, err := readGameplay(ctx, tx)
		if err != nil {
			return true, err
		}
		path := []int{}
		target := 0
		for _, candidate := range g.Stars {
			if !hasStation(candidate.ID) {
				continue
			}
			p := g.path(s.Star, candidate.ID)
			if len(p) > 0 && (target == 0 || len(p) < len(path)) {
				path = p
				target = candidate.ID
			}
		}
		if target == 0 {
			return true, errors.New("В этом компоненте карты нет доступного порта")
		}
		tag, err := tx.Exec(ctx, `UPDATE users SET credits=credits-$2 WHERE id=$1 AND credits>=$2`, u.ID, settings.RescueCost)
		if err != nil {
			return true, err
		}
		if tag.RowsAffected() == 0 {
			return true, errors.New("Недостаточно кредитов")
		}
		route, _ := json.Marshal(path)
		_, err = tx.Exec(ctx, `UPDATE ships SET destination=$2,departure=now(),arrival=now()+interval '30 seconds',travel_path=$3,waypoints='[]',reserved_fuel=0,fuel=GREATEST(fuel,10) WHERE id=$1`, s.ID, target, route)
		return true, err
	case "buy_ship":
		return true, buyShip(ctx, tx, u, *s, in)
	case "extractor":
		if err := idle(ctx, tx, *s); err != nil {
			return true, err
		}
		if !docked(*s) {
			return true, errors.New("Установка доступна в порту")
		}
		if !strings.Contains("|ore|ice|gas|universal|", "|"+in.Target+"|") {
			return true, errors.New("Неизвестный добытчик")
		}
		tag, err := tx.Exec(ctx, `UPDATE users SET credits=credits-250 WHERE id=$1 AND credits>=250`, u.ID)
		if err != nil {
			return true, err
		}
		if tag.RowsAffected() == 0 {
			return true, errors.New("Недостаточно кредитов")
		}
		_, err = tx.Exec(ctx, `UPDATE ships SET extractor=$2 WHERE id=$1`, s.ID, in.Target)
		return true, err
	case "portal_contribute", "portal_travel":
		return true, a.portalCommand(ctx, tx, u, s, g, in)
	}
	return false, nil
}
func (a *App) notifications(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.Method == "POST" {
		var in Command
		if !decode(w, r, &in) {
			return
		}
		if len(in.IDs) > 500 {
			fail(w, 400, "Слишком много записей")
			return
		}
		var err error
		switch in.Action {
		case "delete":
			_, err = a.db.Exec(ctx, `DELETE FROM notifications WHERE user_id=$1 AND id=ANY($2::bigint[])`, u.ID, in.IDs)
		case "clear":
			_, err = a.db.Exec(ctx, `DELETE FROM notifications WHERE user_id=$1`, u.ID)
		case "seen":
			_, err = a.db.Exec(ctx, `UPDATE notifications SET seen=TRUE WHERE user_id=$1 AND (cardinality($2::bigint[])=0 OR id=ANY($2::bigint[]))`, u.ID, in.IDs)
		case "add":
			if len(in.Message) > 500 || !strings.Contains("|success|error|warning|", "|"+in.Level+"|") {
				fail(w, 400, "Некорректное уведомление")
				return
			}
			// Older clients persisted these transient acknowledgements on every click.
			switch strings.TrimSpace(in.Message) {
			case "Действие выполнено", "Тик выполнен", "Action completed", "Tick advanced":
				break
			default:
				_, err = a.db.Exec(ctx, `INSERT INTO notifications(user_id,kind,level,message) VALUES($1,'message',$2,$3)`, u.ID, in.Level, in.Message)
			}
		default:
			fail(w, 400, "Неизвестное действие")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	items, err := queryMaps(ctx, a.db, `SELECT * FROM notifications WHERE user_id=$1 ORDER BY id DESC LIMIT 500`, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	var count, unread int
	if err = a.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE NOT seen) FROM notifications WHERE user_id=$1`, u.ID).Scan(&count, &unread); err != nil {
		a.internal(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	reply(w, 200, map[string]any{"items": items, "count": count, "unread": unread})
}
