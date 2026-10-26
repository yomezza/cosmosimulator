package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Site struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Kind        string           `json:"kind"`
	Orbit       float64          `json:"orbit"`
	Resources   []string         `json:"resources"`
	Planet      int              `json:"planet"`
	Details     *Planet          `json:"details,omitempty"`
	Quotas      map[string]Quota `json:"quotas,omitempty"`
	SharedQuota *Quota           `json:"shared_quota,omitempty"`
}

func hasStation(id int) bool { return id%3 != 0 || id == 1 || id == 5 }
func systemSites(s Star) []Site {
	sites := []Site{}
	scale := math.Sqrt(math.Max(s.Luminosity, .001))
	for _, p := range s.Planets {
		sites = append(sites, Site{ID: fmt.Sprintf("planet-%d", p.ID), Name: p.Name, Kind: p.Kind, Orbit: p.AU, Resources: p.Resources, Planet: p.ID, Details: &p})
	}
	sites = append(sites, Site{ID: "asteroids", Name: s.Name + " · A", Kind: "Пояс астероидов", Orbit: 2.3 * scale, Resources: variedResources(s.ID, 101, "Пояс астероидов"), Planet: 101})
	if s.ID%2 == 0 {
		sites = append(sites, Site{ID: "ice", Name: s.Name + " · I", Kind: "Ледяное поле", Orbit: 6.5 * scale, Resources: variedResources(s.ID, 102, "Ледяное поле"), Planet: 102})
	}
	if hasStation(s.ID) {
		for i := 0; i < 1+s.ID%3; i++ {
			id := "station"
			if i > 0 {
				id = fmt.Sprintf("station-%d", i+1)
			}
			sites = append(sites, Site{ID: id, Name: fmt.Sprintf("%s · PORT %d", s.Name, i+1), Kind: "Орбитальная станция", Orbit: (1.2 + float64(i)*.8) * scale})
		}
	}
	return sites
}
func (a *App) systemView(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	g, loadErr := a.loadUniverse(r.Context())
	if loadErr != nil {
		a.internal(w, loadErr)
		return
	}
	if err != nil || id < 1 || id > len(g.Stars) {
		fail(w, 400, "Некорректная система")
		return
	}
	s := g.Stars[id-1]
	var scanned bool
	if err = a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM surveys WHERE user_id=$1 AND star=$2)`, u.ID, id).Scan(&scanned); err != nil {
		a.internal(w, err)
		return
	}
	// Distant objects are visible as unverified orbital contacts; deposits need a local survey.
	sites := systemSites(s)
	if !scanned {
		for i := range sites {
			sites[i].Resources = nil
			if sites[i].Details != nil {
				sites[i].Details.Resources = nil
			}
		}
	}
	if scanned {
		tx, err := a.db.Begin(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		defer tx.Rollback(context.Background())
		sites, err = quotaView(r.Context(), tx, s, sites)
		if err != nil {
			a.internal(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			a.internal(w, err)
			return
		}
	}
	s.Planets = nil
	reply(w, 200, map[string]any{"star": s, "region": regionName(s.Region), "surveyed": scanned, "sites": sites})
}
func (a *App) profile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	var level, xp int
	var company *int64
	var name *string
	var tag, color, flag *string
	err := a.db.QueryRow(r.Context(), `SELECT u.level,u.xp,u.company_id,c.name,c.tag,c.color,c.flag FROM users u LEFT JOIN companies c ON c.id=u.company_id WHERE u.id=$1`, u.ID).Scan(&level, &xp, &company, &name, &tag, &color, &flag)
	if err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]any{"user": u, "level": level, "xp": xp, "required_xp": level * 100, "upgrade_cost": level * 100, "company_id": company, "company": name, "company_tag": tag, "company_color": color, "company_flag": flag})
}
func (a *App) upgrade(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	tag, err := a.db.Exec(r.Context(), `WITH changed AS(UPDATE users SET xp=xp-level*100,credits=credits-level*100,level=level+1 WHERE id=$1 AND level<100 AND xp>=level*100 AND credits>=level*100 RETURNING id,level) INSERT INTO game_events(user_id,kind,data) SELECT id,'level_up',jsonb_build_object('level',level) FROM changed`, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Недостаточно опыта или кредитов")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) adminTick(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	if err := a.advanceTick(r.Context(), true, u.ID); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) editUser(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	var in struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Credits  int64  `json:"credits"`
		Level    int    `json:"level"`
		XP       int    `json:"xp"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Role != "admin" && in.Role != "player") || in.Credits < 0 || in.Credits > 1000000000 || in.Level < 1 || in.Level > 100 || in.XP < 0 || in.XP > 1000000 {
		fail(w, 400, "Некорректные данные пользователя")
		return
	}
	var hash []byte
	var err error
	if in.Password != "" {
		if len(in.Password) < 12 || len(in.Password) > 72 {
			fail(w, 400, "Пароль: 12–72 байта")
			return
		}
		hash, err = bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(784217)`); err != nil {
		a.internal(w, err)
		return
	}
	var role string
	if err = tx.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, u.ID).Scan(&role); err != nil || role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	var bot bool
	var oldName string
	var oldRole string
	err = tx.QueryRow(r.Context(), `SELECT is_bot,username,role FROM users WHERE id=$1 FOR UPDATE`, in.ID).Scan(&bot, &oldName, &oldRole)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Пользователь не найден")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if bot && (in.Username != oldName || in.Role != oldRole || len(hash) > 0) {
		fail(w, 400, "Нельзя изменять идентификацию бота")
		return
	}
	if !bot && !validName(in.Username) {
		fail(w, 400, "Некорректное имя пользователя")
		return
	}
	if oldRole == "admin" && in.Role != "admin" {
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE role='admin'`).Scan(&count); err != nil {
			a.internal(w, err)
			return
		}
		if count <= 1 || in.ID == u.ID {
			fail(w, 409, "Нельзя понизить себя или последнего администратора")
			return
		}
	}
	tag, err := tx.Exec(r.Context(), `UPDATE users SET username=$2,role=$3,credits=$4,level=$5,xp=$6,password_hash=CASE WHEN $7='' THEN password_hash ELSE $7 END WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM users WHERE username=$2 AND id<>$1)`, in.ID, in.Username, in.Role, in.Credits, in.Level, in.XP, string(hash))
	if err != nil {
		a.internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Имя уже занято")
		return
	}
	if len(hash) > 0 || oldRole != in.Role {
		if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, in.ID); err != nil {
			a.internal(w, err)
			return
		}
	}
	audit, _ := json.Marshal(map[string]any{"user": in.ID, "role": in.Role, "credits": in.Credits, "level": in.Level, "xp": in.XP, "password_reset": len(hash) > 0})
	if _, err = tx.Exec(r.Context(), `INSERT INTO admin_log(actor,action,data) VALUES($1,'user_edit',$2)`, u.ID, audit); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

type Company struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Color   string `json:"color"`
	Flag    string `json:"flag"`
	Owner   *int64 `json:"owner"`
	Starter bool   `json:"starter"`
	Members int    `json:"members"`
}

func validCompanyTag(s string) bool {
	runes := []rune(s)
	if len(runes) < 3 || len(runes) > 4 {
		return false
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (a *App) companyList(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	scope := r.URL.Query().Get("scope")
	var star int
	err := a.db.QueryRow(r.Context(), `SELECT COALESCE((SELECT CASE WHEN arrival<=now() THEN destination ELSE star END FROM ships WHERE user_id=$1),1)`, u.ID).Scan(&star)
	if err != nil {
		a.internal(w, err)
		return
	}
	g, err := a.loadUniverse(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT c.id,c.name,c.tag,c.color,c.flag,c.owner_id,c.starter,count(u.id)::int FROM companies c LEFT JOIN users u ON u.company_id=c.id GROUP BY c.id ORDER BY c.starter DESC,c.id LIMIT 500`)
	if err != nil {
		a.internal(w, err)
		return
	}
	list := []Company{}
	for rows.Next() {
		var c Company
		if err = rows.Scan(&c.ID, &c.Name, &c.Tag, &c.Color, &c.Flag, &c.Owner, &c.Starter, &c.Members); err != nil {
			rows.Close()
			a.internal(w, err)
			return
		}
		list = append(list, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		a.internal(w, err)
		return
	}
	if scope == "system" || scope == "region" {
		filtered := []Company{}
		for _, c := range list {
			var exists bool
			stars := []int{star}
			if scope == "region" {
				stars = nil
				for _, s := range g.Stars {
					if s.Region == g.Stars[star-1].Region {
						stars = append(stars, s.ID)
					}
				}
			}
			if err = a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users u LEFT JOIN ships s ON s.user_id=u.id WHERE u.company_id=$1 AND COALESCE(CASE WHEN s.arrival<=now() THEN s.destination ELSE s.star END,1)=ANY($2::int[]) AND (s.destination IS NULL OR s.arrival<=now()))`, c.ID, stars).Scan(&exists); err != nil {
				a.internal(w, err)
				return
			}
			if exists {
				filtered = append(filtered, c)
			}
		}
		list = filtered
	}
	reply(w, 200, list)
}
func (a *App) companyAction(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Tag    string `json:"tag"`
		Color  string `json:"color"`
		Flag   string `json:"flag"`
	}
	if !decode(w, r, &in) {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var current *int64
	if err = tx.QueryRow(r.Context(), `SELECT company_id FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&current); err != nil {
		a.internal(w, err)
		return
	}
	problem := ""
	switch in.Action {
	case "leave":
		if current != nil {
			var owner *int64
			if err = tx.QueryRow(r.Context(), `SELECT owner_id FROM companies WHERE id=$1 FOR UPDATE`, *current).Scan(&owner); err != nil {
				break
			}
			if owner != nil && *owner == u.ID {
				problem = "Владелец должен оставаться в компании"
				break
			}
		}
		_, err = tx.Exec(r.Context(), `UPDATE users SET company_id=NULL WHERE id=$1`, u.ID)
	case "join":
		if current != nil {
			problem = "Сначала выйдите из текущей компании"
			break
		}
		var id int64
		err = tx.QueryRow(r.Context(), `SELECT id FROM companies WHERE id=$1 FOR KEY SHARE`, in.ID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
			problem = "Компания не найдена"
			break
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE users SET company_id=$2 WHERE id=$1`, u.ID, id)
		}
	case "create":
		if current != nil {
			problem = "Сначала выйдите из текущей компании"
			break
		}
		in.Name = strings.TrimSpace(in.Name)
		in.Tag = strings.ToUpper(in.Tag)
		if len([]rune(in.Name)) < 3 || len([]rune(in.Name)) > 60 || !validCompanyTag(in.Tag) || !colorPattern.MatchString(in.Color) || !strings.Contains("◈◆★☀☾⚑⚙✦", in.Flag) || in.Flag == "" || len([]rune(in.Flag)) != 1 {
			problem = "Некорректные параметры компании"
			break
		}
		var id int64
		err = tx.QueryRow(r.Context(), `INSERT INTO companies(name,tag,color,flag,owner_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING id`, in.Name, in.Tag, in.Color, in.Flag, u.ID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
			problem = "Название или сокращение уже занято"
			break
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE users SET company_id=$2 WHERE id=$1`, u.ID, id)
		}
	default:
		problem = "Неизвестное действие"
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if problem != "" {
		fail(w, 409, problem)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

// Reservations prepay stock held at the seller's station. No teleportation of cargo.
func (a *App) globalMarket(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if r.Method == "GET" {
		g, err := a.loadUniverse(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		rows, err := a.db.Query(r.Context(), `SELECT star,credits,stock FROM local_markets ORDER BY star LIMIT 1000`)
		if err != nil {
			a.internal(w, err)
			return
		}
		ports := []map[string]any{}
		for rows.Next() {
			var star int
			var credits int64
			var raw []byte
			if err = rows.Scan(&star, &credits, &raw); err != nil {
				rows.Close()
				a.internal(w, err)
				return
			}
			if !hasStation(star) {
				continue
			}
			var stock map[string]int
			if err = json.Unmarshal(raw, &stock); err != nil {
				rows.Close()
				a.internal(w, err)
				return
			}
			ports = append(ports, map[string]any{"star": star, "credits": credits, "offers": offers(g.Stars[star-1], stock)})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			a.internal(w, err)
			return
		}
		seen := map[int]bool{}
		for _, port := range ports {
			seen[port["star"].(int)] = true
		}
		for _, s := range g.Stars {
			if hasStation(s.ID) && !seen[s.ID] {
				ports = append(ports, map[string]any{"star": s.ID, "credits": int64(5000), "offers": offers(s, marketStock(s))})
			}
		}
		rows, err = a.db.Query(r.Context(), `SELECT id,star,resource,quantity,cost,status FROM reservations WHERE user_id=$1 ORDER BY (status='reserved') DESC,id DESC LIMIT 200`, u.ID)
		if err != nil {
			a.internal(w, err)
			return
		}
		defer rows.Close()
		reservations := []map[string]any{}
		for rows.Next() {
			var id, cost int64
			var star, q int
			var resource, status string
			if err = rows.Scan(&id, &star, &resource, &q, &cost, &status); err != nil {
				a.internal(w, err)
				return
			}
			reservations = append(reservations, map[string]any{"id": id, "star": star, "resource": resource, "quantity": q, "cost": cost, "status": status})
		}
		if rows.Err() != nil {
			a.internal(w, rows.Err())
			return
		}
		reply(w, 200, map[string]any{"ports": ports, "reservations": reservations})
		return
	}
	var in struct {
		Action   string `json:"action"`
		Star     int    `json:"star"`
		Resource string `json:"resource"`
		Quantity int    `json:"quantity"`
		ID       int64  `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	g, err := a.loadUniverse(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var credits int64
	if err = tx.QueryRow(r.Context(), `SELECT credits FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&credits); err != nil {
		a.internal(w, err)
		return
	}
	ship, err := ensureShip(r.Context(), tx, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	reservationID := in.ID
	var cost int64
	var star, q int
	var res string
	problem := ""
	if in.Action == "reserve" {
		star, q, res = in.Star, in.Quantity, in.Resource
		var outstanding int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM reservations WHERE user_id=$1 AND status='reserved'`, u.ID).Scan(&outstanding); err != nil {
			a.internal(w, err)
			return
		}
		if outstanding >= 100 {
			fail(w, 409, "Достигнут лимит активных броней")
			return
		}
		if star < 1 || star > len(g.Stars) || !hasStation(star) || q < 1 || q > 60 {
			fail(w, 400, "Некорректная сделка")
			return
		}
		if _, ok := resource(res); !ok {
			fail(w, 400, "Некорректная сделка")
			return
		}
		stock := marketStock(g.Stars[star-1])
		raw, _ := json.Marshal(stock)
		if _, err = tx.Exec(r.Context(), `INSERT INTO local_markets(star,stock) VALUES($1,$2) ON CONFLICT DO NOTHING`, star, raw); err != nil {
			a.internal(w, err)
			return
		}
		var wallet int64
		if err = tx.QueryRow(r.Context(), `SELECT credits,stock FROM local_markets WHERE star=$1 FOR UPDATE`, star).Scan(&wallet, &raw); err != nil {
			a.internal(w, err)
			return
		}
		if err = json.Unmarshal(raw, &stock); err != nil {
			a.internal(w, err)
			return
		}
		for _, o := range offers(g.Stars[star-1], stock) {
			if o.Resource == res {
				cost = int64(q * o.Buy)
			}
		}
		if stock[res] < q {
			problem = "Нет товара на рынке"
		} else if credits < cost {
			problem = "Недостаточно кредитов"
		} else {
			stock[res] -= q
			raw, _ = json.Marshal(stock)
			if _, err = tx.Exec(r.Context(), `UPDATE local_markets SET stock=$2 WHERE star=$1`, star, raw); err == nil {
				err = tx.QueryRow(r.Context(), `INSERT INTO reservations(user_id,star,resource,quantity,cost) VALUES($1,$2,$3,$4,$5) RETURNING id`, u.ID, star, res, q, cost).Scan(&reservationID)
			}
			credits -= cost
		}
	} else if in.Action == "collect" || in.Action == "cancel" {
		var status string
		err = tx.QueryRow(r.Context(), `SELECT star,resource,quantity,cost,status FROM reservations WHERE id=$1 AND user_id=$2 FOR UPDATE`, in.ID, u.ID).Scan(&star, &res, &q, &cost, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Бронь не найдена")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if status != "reserved" {
			problem = "Бронь уже обработана"
		} else if in.Action == "collect" {
			var pending int
			if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(quantity),0) FROM mining_jobs WHERE user_id=$1`, u.ID).Scan(&pending); err != nil {
				a.internal(w, err)
				return
			}
			if !docked(ship) || ship.Star != star {
				problem = "Прибудьте на станцию получения"
			} else if cargoSize(ship.Cargo)+pending+q > 60 {
				problem = "Трюм заполнен"
			} else {
				ship.Cargo[res] += q
				raw, _ := json.Marshal(ship.Cargo)
				if _, err = tx.Exec(r.Context(), `UPDATE ships SET cargo=$2 WHERE user_id=$1`, u.ID, raw); err == nil {
					_, err = tx.Exec(r.Context(), `UPDATE local_markets SET credits=credits+$2 WHERE star=$1`, star, cost)
				}
				if err == nil {
					_, err = tx.Exec(r.Context(), `UPDATE reservations SET status='collected' WHERE id=$1`, in.ID)
				}
			}
		} else {
			_, err = tx.Exec(r.Context(), `UPDATE local_markets SET stock=jsonb_set(stock,ARRAY[$2::text],to_jsonb(COALESCE((stock->>$2)::int,0)+$3)) WHERE star=$1`, star, res, q)
			if err == nil {
				_, err = tx.Exec(r.Context(), `UPDATE reservations SET status='cancelled' WHERE id=$1`, in.ID)
			}
			credits += cost
		}
	} else {
		problem = "Неизвестное действие"
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if problem != "" {
		fail(w, 409, problem)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE users SET credits=$2 WHERE id=$1`, u.ID, credits); err != nil {
		a.internal(w, err)
		return
	}
	if err = recordEvent(r.Context(), tx, u.ID, "reservation_"+in.Action, map[string]any{"reservation": reservationID, "star": star, "resource": res, "quantity": q, "cost": cost}); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

type MiningJob struct {
	Resource   string    `json:"resource"`
	Quantity   int       `json:"quantity"`
	FinishTick int64     `json:"finish_tick"`
	StartedAt  time.Time `json:"started_at"`
	Planet     int       `json:"planet"`
}

func completeMining(ctx context.Context, tx pgx.Tx, tick int64) error {
	rows, err := tx.Query(ctx, `SELECT user_id,resource,quantity FROM mining_jobs WHERE finish_tick<=$1 ORDER BY user_id`, tick)
	if err != nil {
		return err
	}
	type result struct {
		id       int64
		resource string
		q        int
	}
	jobs := []result{}
	for rows.Next() {
		var j result
		if err = rows.Scan(&j.id, &j.resource, &j.q); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if err = recordEvent(ctx, tx, j.id, "mining_complete", map[string]any{"resource": j.resource, "quantity": j.q}); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE ships SET cargo=jsonb_set(cargo,ARRAY[$2::text],to_jsonb(COALESCE((cargo->>$2)::int,0)+$3)) WHERE user_id=$1`, j.id, j.resource, j.q); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET xp=xp+5 WHERE id=$1`, j.id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM mining_jobs WHERE user_id=$1`, j.id); err != nil {
			return err
		}
	}
	return nil
}
