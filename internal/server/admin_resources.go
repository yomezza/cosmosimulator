package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
)

func (a *App) adminResources(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	ctx := r.Context()
	if r.Method == "GET" {
		uid, _ := strconv.ParseInt(r.URL.Query().Get("user"), 10, 64)
		if uid == 0 {
			uid = u.ID
		}
		ships, err := queryMaps(ctx, a.db, `SELECT id,name,class,star,object_id,cargo FROM ships WHERE user_id=$1 ORDER BY id`, uid)
		if err != nil {
			a.internal(w, err)
			return
		}
		companies, err := queryMaps(ctx, a.db, `SELECT id,name,tag FROM companies ORDER BY founder DESC,starter DESC,name`)
		if err != nil {
			a.internal(w, err)
			return
		}
		reply(w, 200, map[string]any{"ships": ships, "companies": companies})
		return
	}
	var in struct {
		User     int64  `json:"user"`
		Ship     int64  `json:"ship"`
		Company  int64  `json:"company"`
		Star     int    `json:"star"`
		Station  string `json:"station"`
		Target   string `json:"target"`
		Resource string `json:"resource"`
		Quantity int    `json:"quantity"`
	}
	if !decode(w, r, &in) {
		return
	}
	if _, ok := resource(in.Resource); !ok || in.Quantity < 1 || in.Quantity > 1000000 {
		fail(w, 400, "Некорректный ресурс или количество")
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
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&role); err != nil || role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, in.User).Scan(&in.User); errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Пользователь не найден")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	before, after := 0, 0
	switch in.Target {
	case "ship":
		s, e := readShip(ctx, tx, in.Ship)
		if e != nil {
			fail(w, 404, "Корабль не найден")
			return
		}
		var owner int64
		if err = tx.QueryRow(ctx, `SELECT user_id FROM ships WHERE id=$1`, s.ID).Scan(&owner); err != nil || owner != in.User {
			fail(w, 404, "Корабль не найден")
			return
		}
		var reserved int
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity),0) FROM mining_jobs WHERE ship_id=$1`, s.ID).Scan(&reserved); err != nil {
			a.internal(w, err)
			return
		}
		if cargoSize(s.Cargo)+reserved+in.Quantity > s.capacity() {
			fail(w, 409, "Трюм заполнен")
			return
		}
		before = s.Cargo[in.Resource]
		s.Cargo[in.Resource] += in.Quantity
		after = s.Cargo[in.Resource]
		err = saveShip(ctx, tx, in.User, s)
	case "warehouse", "company":
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stations WHERE star=$1 AND object_id=$2 AND active)`, in.Star, in.Station).Scan(&exists); err != nil {
			a.internal(w, err)
			return
		}
		if !exists {
			fail(w, 404, "Станция не найдена")
			return
		}
		var cargo map[string]int
		if in.Target == "warehouse" {
			cargo, err = warehouse(ctx, tx, in.User, in.Star, in.Station)
		} else {
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM companies WHERE id=$1)`, in.Company).Scan(&exists); err != nil || !exists {
				fail(w, 404, "Корпорация не найдена")
				return
			}
			if _, err = tx.Exec(ctx, `INSERT INTO company_storage(company_id,star,station) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, in.Company, in.Star, in.Station); err == nil {
				var raw []byte
				err = tx.QueryRow(ctx, `SELECT cargo FROM company_storage WHERE company_id=$1 AND star=$2 AND station=$3 FOR UPDATE`, in.Company, in.Star, in.Station).Scan(&raw)
				if err == nil {
					err = json.Unmarshal(raw, &cargo)
				}
			}
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		before = cargo[in.Resource]
		if before > 1000000000-in.Quantity {
			fail(w, 409, "Склад заполнен")
			return
		}
		cargo[in.Resource] += in.Quantity
		after = cargo[in.Resource]
		if in.Target == "warehouse" {
			err = putWarehouse(ctx, tx, in.User, in.Star, in.Station, cargo)
		} else {
			raw, _ := json.Marshal(cargo)
			_, err = tx.Exec(ctx, `UPDATE company_storage SET cargo=$4 WHERE company_id=$1 AND star=$2 AND station=$3`, in.Company, in.Star, in.Station, raw)
		}
	default:
		fail(w, 400, "Неизвестное место выдачи")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	raw, _ := json.Marshal(map[string]any{"user": in.User, "ship": in.Ship, "company": in.Company, "star": in.Star, "station": in.Station, "target": in.Target, "resource": in.Resource, "quantity": in.Quantity, "before": before, "after": after})
	if _, err = tx.Exec(ctx, `INSERT INTO admin_log(actor,action,data) VALUES($1,'resource_grant',$2)`, u.ID, raw); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
