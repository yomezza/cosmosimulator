package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type CargoLocation struct {
	Kind    string `json:"kind"`
	Ship    int64  `json:"ship,omitempty"`
	Star    int    `json:"star,omitempty"`
	Station string `json:"station,omitempty"`
	Company int64  `json:"company,omitempty"`
}

type CargoItem struct {
	Resource string `json:"resource"`
	Quantity int    `json:"quantity"`
}

func storageAccess(ctx context.Context, tx pgx.Tx, uid int64, loc CargoLocation, withdraw bool) (map[string]int, error) {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stations WHERE star=$1 AND object_id=$2 AND active)`, loc.Star, loc.Station).Scan(&active); err != nil {
		return nil, err
	}
	if !active {
		return nil, errors.New("Станция не найдена")
	}
	if loc.Kind == "personal" {
		return warehouse(ctx, tx, uid, loc.Star, loc.Station)
	}
	if loc.Kind != "company" {
		return nil, errors.New("Неизвестный склад")
	}
	permission := "member"
	if withdraw {
		permission = "warehouse"
	}
	cid, allowed, err := companyAccess(ctx, tx, uid, permission)
	if err != nil {
		return nil, err
	}
	if !withdraw {
		allowed = cid > 0
	}
	if !allowed || cid != loc.Company {
		return nil, errors.New("Недостаточно корпоративных полномочий")
	}
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM company_projects WHERE company_id=$1 AND kind='warehouse' AND star=$2 AND station=$3 AND status='complete')`, cid, loc.Star, loc.Station).Scan(&enabled); err != nil {
		return nil, err
	}
	if !enabled {
		return nil, errors.New("Здесь нет корпоративного склада")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO company_storage(company_id,star,station) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, cid, loc.Star, loc.Station); err != nil {
		return nil, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT cargo FROM company_storage WHERE company_id=$1 AND star=$2 AND station=$3 FOR UPDATE`, cid, loc.Star, loc.Station).Scan(&raw); err != nil {
		return nil, err
	}
	m := map[string]int{}
	err = json.Unmarshal(raw, &m)
	return m, err
}
func writeStorage(ctx context.Context, tx pgx.Tx, uid int64, loc CargoLocation, m map[string]int) error {
	if loc.Kind == "personal" {
		return putWarehouse(ctx, tx, uid, loc.Star, loc.Station, m)
	}
	raw, _ := json.Marshal(m)
	_, err := tx.Exec(ctx, `UPDATE company_storage SET cargo=$4 WHERE company_id=$1 AND star=$2 AND station=$3`, loc.Company, loc.Star, loc.Station, raw)
	return err
}
func transferLocation(ctx context.Context, tx pgx.Tx, uid int64, loc CargoLocation, withdraw bool) (CargoLocation, *Ship, map[string]int, error) {
	if loc.Kind != "ship" {
		loc.Ship = 0
		if loc.Kind == "personal" {
			loc.Company = 0
		}
		m, err := storageAccess(ctx, tx, uid, loc, withdraw)
		return loc, nil, m, err
	}
	var owner int64
	if err := tx.QueryRow(ctx, `SELECT user_id FROM ships WHERE id=$1`, loc.Ship).Scan(&owner); err != nil || owner != uid {
		return loc, nil, nil, errors.New("Корабль не найден")
	}
	loc.Company = 0
	s, err := readShip(ctx, tx, loc.Ship)
	if err != nil {
		return loc, nil, nil, err
	}
	loc.Star = s.Star
	loc.Station = s.ObjectID
	return loc, &s, s.Cargo, nil
}
func sameCargoLocation(a, b CargoLocation) bool {
	return a.Kind == b.Kind && a.Ship == b.Ship && a.Star == b.Star && a.Station == b.Station && a.Company == b.Company
}
func logisticsDigest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (a *App) logistics(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.Method == "GET" {
		personal, err := queryMaps(ctx, a.db, `SELECT * FROM warehouses WHERE user_id=$1`, u.ID)
		if err != nil {
			a.internal(w, err)
			return
		}
		cid, canWithdraw, err := companyAccess(ctx, a.db, u.ID, "warehouse")
		if err != nil {
			a.internal(w, err)
			return
		}
		corp, err := queryMaps(ctx, a.db, `SELECT * FROM company_storage WHERE company_id=$1`, cid)
		if err != nil {
			a.internal(w, err)
			return
		}
		built, err := queryMaps(ctx, a.db, `SELECT star,station FROM company_projects WHERE company_id=$1 AND kind='warehouse' AND status='complete'`, cid)
		if err != nil {
			a.internal(w, err)
			return
		}
		history, err := queryMaps(ctx, a.db, `SELECT o.result,o.created_at,t.status AS task_status,t.progress FROM logistics_operations o LEFT JOIN automation_tasks t ON t.id=(o.result->>'task')::bigint WHERE o.user_id=$1 ORDER BY o.created_at DESC LIMIT 20`, u.ID)
		if err != nil {
			a.internal(w, err)
			return
		}
		reply(w, 200, map[string]any{"personal": personal, "corporate": corp, "built": built, "company": cid, "can_withdraw": canWithdraw, "can_deposit": cid > 0, "history": history})
		return
	}
	var in struct {
		Request  string        `json:"request"`
		Mode     string        `json:"mode"`
		From     CargoLocation `json:"from"`
		To       CargoLocation `json:"to"`
		Ship     int64         `json:"ship"`
		Resource string        `json:"resource"`
		Quantity int           `json:"quantity"`
		Items    []CargoItem   `json:"items,omitempty"`
	}
	if !decode(w, r, &in) {
		return
	}
	batch := len(in.Items) > 0
	items := in.Items
	if !batch {
		items = []CargoItem{{in.Resource, in.Quantity}}
	}
	if len(items) > len(resources) || batch && (in.Resource != "" || in.Quantity != 0) || len(in.Request) < 16 || len(in.Request) > 64 || strings.ContainsAny(in.Request, "\r\n\x00") || (in.Mode != "direct" && in.Mode != "queue") {
		fail(w, 400, "Некорректный перенос ресурса")
		return
	}
	seen := map[string]bool{}
	total := 0
	for _, item := range items {
		if _, ok := resource(item.Resource); !ok || item.Quantity < 1 || item.Quantity > 100000 || seen[item.Resource] {
			fail(w, 400, "Некорректный перенос ресурса")
			return
		}
		seen[item.Resource] = true
		total += item.Quantity
	}
	digest := logisticsDigest(in)
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
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, u.ID); err != nil {
		a.internal(w, err)
		return
	}
	var savedDigest string
	var result []byte
	err = tx.QueryRow(ctx, `SELECT digest,result FROM logistics_operations WHERE user_id=$1 AND request_id=$2`, u.ID, in.Request).Scan(&savedDigest, &result)
	if err == nil {
		if savedDigest != digest {
			fail(w, 409, "Этот запрос уже использован для другого переноса")
			return
		}
		reply(w, 200, json.RawMessage(result))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	from, fromShip, source, err := transferLocation(ctx, tx, u.ID, in.From, true)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	to, toShip, target, err := transferLocation(ctx, tx, u.ID, in.To, false)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	if sameCargoLocation(from, to) {
		fail(w, 400, "Выберите другое место хранения")
		return
	}
	for _, item := range items {
		if source[item.Resource] < item.Quantity {
			fail(w, 409, "Недостаточно груза")
			return
		}
	}
	if toShip != nil {
		var reserved int
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity),0) FROM mining_jobs WHERE ship_id=$1`, toShip.ID).Scan(&reserved); err != nil {
			a.internal(w, err)
			return
		}
		if cargoSize(toShip.Cargo)+reserved+total > toShip.capacity() {
			fail(w, 409, "Трюм заполнен")
			return
		}
	} else {
		for _, item := range items {
			if target[item.Resource] > 1000000000-item.Quantity {
				fail(w, 409, "Склад заполнен")
				return
			}
		}
	}
	var carrier Ship
	if fromShip != nil {
		carrier = *fromShip
	} else if toShip != nil {
		carrier = *toShip
	} else {
		var owner int64
		if err = tx.QueryRow(ctx, `SELECT user_id FROM ships WHERE id=$1`, in.Ship).Scan(&owner); err != nil || owner != u.ID {
			fail(w, 404, "Корабль не найден")
			return
		}
		carrier, err = readShip(ctx, tx, in.Ship)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	taskID := int64(0)
	taskIDs := []int64{}
	if in.Mode == "direct" {
		if !docked(carrier) || carrier.Star != from.Star || carrier.ObjectID != from.Station || from.Star != to.Star || from.Station != to.Station || fromShip != nil && !docked(*fromShip) || toShip != nil && !docked(*toShip) {
			fail(w, 409, "Для прямого переноса нужна стыковка с этой станцией")
			return
		}
		for _, sid := range []int64{carrier.ID, from.Ship, to.Ship} {
			if sid == 0 {
				continue
			}
			if busy, e := automationBusy(ctx, tx, sid); e != nil {
				a.internal(w, e)
				return
			} else if busy {
				fail(w, 409, "Приостановите задачу планировщика")
				return
			}
		}
		for _, item := range items {
			source[item.Resource] -= item.Quantity
			target[item.Resource] += item.Quantity
		}
		if fromShip != nil {
			err = saveShip(ctx, tx, u.ID, *fromShip)
		} else {
			err = writeStorage(ctx, tx, u.ID, from, source)
		}
		if err == nil {
			if toShip != nil {
				err = saveShip(ctx, tx, u.ID, *toShip)
			} else {
				err = writeStorage(ctx, tx, u.ID, to, target)
			}
		}
	} else {
		if fromShip != nil && toShip != nil {
			fail(w, 409, "Для передачи между кораблями нужна общая станция")
			return
		}
		g, e := a.loadUniverse(ctx)
		if e != nil {
			a.internal(w, e)
			return
		}
		if len(g.path(carrier.Star, from.Star)) == 0 || len(g.path(from.Star, to.Star)) == 0 {
			fail(w, 409, "Нет допустимого маршрута")
			return
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM automation_tasks WHERE user_id=$1 AND status IN('queued','running','paused')`, u.ID).Scan(&count); err != nil {
			a.internal(w, err)
			return
		}
		if count+len(items) > 20 {
			fail(w, 409, "В очереди уже 20 задач")
			return
		}
		kind := "haul"
		if fromShip != nil {
			kind = "unload"
		}
		if toShip != nil {
			kind = "load"
		}
		company := from.Company
		if to.Kind == "company" {
			company = to.Company
		}
		var companyID *int64
		if company > 0 {
			companyID = &company
		}
		for _, item := range items {
			err = tx.QueryRow(ctx, `INSERT INTO automation_tasks(user_id,ship_id,kind,source_star,source_object,dest_star,dest_station,resource,quantity,source_kind,dest_kind,company_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, u.ID, carrier.ID, kind, from.Star, from.Station, to.Star, to.Station, item.Resource, item.Quantity, from.Kind, to.Kind, companyID).Scan(&taskID)
			if err != nil {
				break
			}
			taskIDs = append(taskIDs, taskID)
		}
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	result, _ = json.Marshal(map[string]any{"from": from, "to": to, "resource": in.Resource, "quantity": in.Quantity, "mode": in.Mode, "task": taskID, "time": time.Now()})
	if batch {
		result, _ = json.Marshal(map[string]any{"from": from, "to": to, "items": items, "mode": in.Mode, "tasks": taskIDs, "time": time.Now()})
	}
	if _, err = tx.Exec(ctx, `INSERT INTO logistics_operations(user_id,request_id,digest,result) VALUES($1,$2,$3,$4)`, u.ID, in.Request, digest, result); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, json.RawMessage(result))
}
