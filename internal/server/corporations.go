package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
	"unicode/utf8"
)

func companyAccess(ctx context.Context, q querier, uid int64, permission string) (int64, bool, error) {
	var cid *int64
	var owner *int64
	var raw []byte
	err := q.QueryRow(ctx, `SELECT u.company_id,c.owner_id,COALESCE(r.permissions,'[]'::jsonb) FROM users u LEFT JOIN companies c ON c.id=u.company_id LEFT JOIN company_roles r ON r.id=u.company_role AND r.company_id=u.company_id WHERE u.id=$1`, uid).Scan(&cid, &owner, &raw)
	if err != nil {
		return 0, false, err
	}
	if cid == nil {
		return 0, false, nil
	}
	if owner != nil && *owner == uid {
		return *cid, true, nil
	}
	var permissions []string
	if err = json.Unmarshal(raw, &permissions); err != nil {
		return *cid, false, err
	}
	for _, p := range permissions {
		if p == permission {
			return *cid, true, nil
		}
	}
	return *cid, false, nil
}
func (a *App) corporationCommand(ctx context.Context, tx pgx.Tx, u User, s *Ship, g *Galaxy, in Command) (bool, error) {
	var permission string
	switch in.Action {
	case "company_deposit":
		permission = "member"
	case "company_withdraw":
		permission = "finance"
	case "role_save", "role_delete", "role_assign", "company_transfer", "company_dissolve":
		permission = "owner"
	case "invite", "request_accept", "request_reject":
		permission = "manage"
	case "project_create":
		permission = "projects"
	case "project_contribute", "company_store":
		permission = "member"
	case "company_cargo_withdraw":
		permission = "warehouse"
	case "apply", "invitation_accept", "request_cancel", "flag_save":
		permission = "member"
	default:
		return false, nil
	}
	cid, allowed, err := companyAccess(ctx, tx, u.ID, permission)
	if err != nil {
		return true, err
	}
	if permission == "member" {
		allowed = cid > 0
	}
	if in.Action == "apply" || in.Action == "invitation_accept" || in.Action == "request_cancel" {
		allowed = true
	}
	if !allowed {
		return true, errors.New("Недостаточно корпоративных полномочий")
	}
	if cid > 0 {
		if _, err = tx.Exec(ctx, `SELECT id FROM companies WHERE id=$1 FOR UPDATE`, cid); err != nil {
			return true, err
		}
	}
	switch in.Action {
	case "company_deposit", "company_withdraw":
		if in.Amount < 1 || in.Amount > 1000000000 {
			return true, errors.New("Некорректная сумма")
		}
		amount := in.Amount
		if in.Action == "company_deposit" {
			if err = debit(ctx, tx, u.ID, amount); err != nil {
				return true, err
			}
			_, err = tx.Exec(ctx, `UPDATE companies SET credits=credits+$2 WHERE id=$1`, cid, amount)
		} else {
			tag, e := tx.Exec(ctx, `UPDATE companies SET credits=credits-$2 WHERE id=$1 AND credits>=$2`, cid, amount)
			if e != nil {
				return true, e
			}
			if tag.RowsAffected() == 0 {
				return true, errors.New("Недостаточно корпоративных средств")
			}
			err = credit(ctx, tx, u.ID, amount)
			amount = -amount
		}
		if err != nil {
			return true, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO company_finance(company_id,actor,amount,reason) VALUES($1,$2,$3,$4)`, cid, u.ID, amount, in.Action)
	case "role_save":
		if utf8.RuneCountInString(strings.TrimSpace(in.Name)) < 1 || utf8.RuneCountInString(in.Name) > 40 || in.Name == "Владелец" || in.Name == "Участник" {
			return true, errors.New("Некорректное название роли")
		}
		for _, p := range in.Permissions {
			if p != "finance" && p != "manage" && p != "projects" && p != "warehouse" {
				return true, errors.New("Неизвестное полномочие")
			}
		}
		raw, _ := json.Marshal(in.Permissions)
		if in.ID > 0 {
			tag, e := tx.Exec(ctx, `UPDATE company_roles SET name=$3,permissions=$4 WHERE id=$1 AND company_id=$2`, in.ID, cid, in.Name, raw)
			err = e
			if e == nil && tag.RowsAffected() == 0 {
				return true, errors.New("Роль не найдена")
			}
		} else {
			var n int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM company_roles WHERE company_id=$1`, cid).Scan(&n); err != nil {
				return true, err
			}
			if n >= 7 {
				return true, errors.New("Максимум 7 настраиваемых ролей")
			}
			_, err = tx.Exec(ctx, `INSERT INTO company_roles(company_id,name,permissions) VALUES($1,$2,$3)`, cid, in.Name, raw)
		}
	case "role_delete":
		_, err = tx.Exec(ctx, `DELETE FROM company_roles WHERE id=$1 AND company_id=$2`, in.ID, cid)
	case "role_assign":
		var role *int64
		if in.Amount > 0 {
			var id int64
			if err = tx.QueryRow(ctx, `SELECT id FROM company_roles WHERE id=$1 AND company_id=$2`, in.Amount, cid).Scan(&id); err != nil {
				return true, err
			}
			role = &id
		}
		tag, e := tx.Exec(ctx, `UPDATE users SET company_role=$3 WHERE id=$1 AND company_id=$2 AND id<>(SELECT owner_id FROM companies WHERE id=$2)`, in.ID, cid, role)
		err = e
		if e == nil && tag.RowsAffected() == 0 {
			return true, errors.New("Участник не найден")
		}
	case "company_transfer":
		var member bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND company_id=$2 AND NOT is_bot)`, in.ID, cid).Scan(&member); err != nil {
			return true, err
		}
		if !member || in.ID == u.ID {
			return true, errors.New("Выберите другого участника")
		}
		_, err = tx.Exec(ctx, `UPDATE companies SET owner_id=$2 WHERE id=$1`, cid, in.ID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE users SET company_role=NULL WHERE id=$1`, in.ID)
		}
	case "company_dissolve":
		var pending bool
		var balance int64
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT credits,'{}'::jsonb,EXISTS(SELECT 1 FROM company_projects WHERE company_id=$1 AND status='active') OR EXISTS(SELECT 1 FROM company_storage s,LATERAL jsonb_each_text(s.cargo) v WHERE company_id=$1 AND v.value::int>0) FROM companies WHERE id=$1`, cid).Scan(&balance, &raw, &pending); err != nil {
			return true, err
		}
		var stock map[string]int
		_ = json.Unmarshal(raw, &stock)
		if pending || cargoSize(stock) > 0 {
			return true, errors.New("Завершите проекты и освободите корпоративный склад")
		}
		if err = credit(ctx, tx, u.ID, balance); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET company_id=NULL,company_role=NULL WHERE company_id=$1`, cid); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `UPDATE stations SET company_id=NULL,founder=(SELECT id FROM companies WHERE founder_key='founder-01') WHERE company_id=$1`, cid); err != nil {
			return true, err
		}
		_, err = tx.Exec(ctx, `DELETE FROM companies WHERE id=$1 AND NOT starter AND NOT founder`, cid)
	case "apply":
		if cid > 0 {
			return true, errors.New("Сначала выйдите из текущей корпорации")
		}
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM companies WHERE id=$1 AND NOT founder)`, in.ID).Scan(&valid); err != nil {
			return true, err
		}
		if !valid {
			return true, errors.New("Корпорация недоступна")
		}
		_, err = tx.Exec(ctx, `INSERT INTO company_requests(company_id,user_id,kind) VALUES($1,$2,'application') ON CONFLICT DO NOTHING`, in.ID, u.ID)
	case "invite":
		var target int64
		if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE (username=$1 OR id::text=$1) AND NOT is_bot`, in.Name).Scan(&target); err != nil {
			return true, errors.New("Игрок не найден")
		}
		_, err = tx.Exec(ctx, `INSERT INTO company_requests(company_id,user_id,kind) VALUES($1,$2,'invitation') ON CONFLICT DO NOTHING`, cid, target)
		if err == nil {
			err = recordEvent(ctx, tx, target, "company_invitation", map[string]any{"company": cid})
		}
	case "request_accept", "invitation_accept":
		var target, company int64
		var kind string
		if err = tx.QueryRow(ctx, `SELECT company_id,user_id,kind FROM company_requests WHERE id=$1 FOR UPDATE`, in.ID).Scan(&company, &target, &kind); err != nil {
			return true, err
		}
		if (in.Action == "request_accept" && (company != cid || kind != "application")) || (in.Action == "invitation_accept" && (target != u.ID || kind != "invitation")) {
			return true, errors.New("Нет полномочий")
		}
		tag, e := tx.Exec(ctx, `UPDATE users SET company_id=$2,company_role=NULL WHERE id=$1 AND company_id IS NULL`, target, company)
		if e != nil {
			return true, e
		}
		if tag.RowsAffected() == 0 {
			return true, errors.New("Игрок уже состоит в корпорации")
		}
		_, err = tx.Exec(ctx, `DELETE FROM company_requests WHERE user_id=$1`, target)
	case "request_reject", "request_cancel":
		_, err = tx.Exec(ctx, `DELETE FROM company_requests WHERE id=$1 AND (company_id=$2 OR user_id=$3)`, in.ID, func() int64 {
			if in.Action == "request_cancel" {
				return 0
			}
			return cid
		}(), u.ID)
	case "flag_save":
		_, owner, e := companyAccess(ctx, tx, u.ID, "owner")
		if e != nil {
			return true, e
		}
		if !owner {
			return true, errors.New("Требуется владелец")
		}
		if !colorPattern.MatchString(in.Color) || len([]rune(in.Flag)) != 1 || !strings.Contains("◈◆★☀☾⚑⚙✦", in.Flag) {
			return true, errors.New("Некорректный флаг")
		}
		for k, v := range in.FlagDesign {
			if k != "background" && k != "symbol" && k != "stripe" {
				return true, errors.New("Некорректный слой")
			}
			if !colorPattern.MatchString(v) {
				return true, errors.New("Некорректный цвет")
			}
		}
		raw, _ := json.Marshal(in.FlagDesign)
		_, err = tx.Exec(ctx, `UPDATE companies SET color=$2,flag=$3,flag_design=$4 WHERE id=$1`, cid, in.Color, in.Flag, raw)
	case "company_store", "company_cargo_withdraw":
		if !docked(*s) || in.Quantity < 1 || in.Quantity > s.capacity() {
			return true, errors.New("Стыкуйтесь с портом и укажите количество")
		}
		var enabled bool
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT cargo FROM company_storage WHERE company_id=$1 AND star=$2 AND station=$3),'{}'::jsonb),EXISTS(SELECT 1 FROM company_projects WHERE company_id=$1 AND kind='warehouse' AND star=$2 AND station=$3 AND status='complete')`, cid, s.Star, s.ObjectID).Scan(&raw, &enabled); err != nil {
			return true, err
		}
		if !enabled {
			return true, errors.New("Здесь нет корпоративного склада")
		}
		stock := map[string]int{}
		if err = json.Unmarshal(raw, &stock); err != nil {
			return true, err
		}
		if in.Action == "company_store" {
			if s.Cargo[in.Resource] < in.Quantity {
				return true, errors.New("Недостаточно груза")
			}
			stock[in.Resource] += in.Quantity
			s.Cargo[in.Resource] -= in.Quantity
		} else {
			var reserved int
			if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity),0) FROM mining_jobs WHERE ship_id=$1`, s.ID).Scan(&reserved); err != nil {
				return true, err
			}
			if stock[in.Resource] < in.Quantity || cargoSize(s.Cargo)+reserved+in.Quantity > s.capacity() {
				return true, errors.New("Недостаточно товара или места")
			}
			stock[in.Resource] -= in.Quantity
			s.Cargo[in.Resource] += in.Quantity
		}
		raw, _ = json.Marshal(stock)
		if _, err = tx.Exec(ctx, `INSERT INTO company_storage(company_id,star,station,cargo) VALUES($1,$2,$3,$4) ON CONFLICT(company_id,star,station) DO UPDATE SET cargo=EXCLUDED.cargo`, cid, s.Star, s.ObjectID, raw); err != nil {
			return true, err
		}
		err = saveShip(ctx, tx, u.ID, *s)
	case "project_create":
		if !docked(*s) || len([]rune(in.Name)) < 3 || len([]rune(in.Name)) > 60 || (in.Target != "warehouse" && in.Target != "port" && in.Target != "station") {
			return true, errors.New("Некорректный проект")
		}
		required := map[string]int{"iron": 200, "silicon": 100, "orichalcum": 20}
		if in.Target == "warehouse" {
			required = map[string]int{"iron": 100, "silicon": 50}
		}
		raw, _ := json.Marshal(required)
		_, err = tx.Exec(ctx, `INSERT INTO company_projects(company_id,name,kind,star,required,station) VALUES($1,$2,$3,$4,$5,$6)`, cid, in.Name, in.Target, s.Star, raw, s.ObjectID)
	case "project_contribute":
		var star int
		var kind, name, status string
		var needRaw, doneRaw []byte
		if err = tx.QueryRow(ctx, `SELECT star,kind,name,status,required,contributed FROM company_projects WHERE id=$1 AND company_id=$2 FOR UPDATE`, in.ID, cid).Scan(&star, &kind, &name, &status, &needRaw, &doneRaw); err != nil {
			return true, err
		}
		if status != "active" || s.Star != star || !docked(*s) || in.Quantity < 1 || in.Quantity > 100000 {
			return true, errors.New("Проект недоступен")
		}
		need, done := map[string]int{}, map[string]int{}
		_ = json.Unmarshal(needRaw, &need)
		_ = json.Unmarshal(doneRaw, &done)
		if need[in.Resource]-done[in.Resource] < in.Quantity {
			return true, errors.New("Вклад превышает оставшуюся потребность")
		}
		m, e := warehouse(ctx, tx, u.ID, s.Star, s.ObjectID)
		if e != nil {
			return true, e
		}
		if m[in.Resource] < in.Quantity {
			return true, errors.New("Недостаточно ресурса на личном складе")
		}
		m[in.Resource] -= in.Quantity
		if err = putWarehouse(ctx, tx, u.ID, s.Star, s.ObjectID, m); err != nil {
			return true, err
		}
		done[in.Resource] += in.Quantity
		raw, _ := json.Marshal(done)
		if _, err = tx.Exec(ctx, `UPDATE company_projects SET contributed=$2 WHERE id=$1`, in.ID, raw); err != nil {
			return true, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO project_contributions(project_id,user_id,resource,quantity) VALUES($1,$2,$3,$4)`, in.ID, u.ID, in.Resource, in.Quantity); err != nil {
			return true, err
		}
		complete := true
		for r, q := range need {
			if done[r] < q {
				complete = false
			}
		}
		if complete {
			if _, err = tx.Exec(ctx, `UPDATE company_projects SET status='complete' WHERE id=$1`, in.ID); err != nil {
				return true, err
			}
			if kind != "warehouse" {
				_, err = tx.Exec(ctx, `INSERT INTO stations(star,object_id,name,specialization,company_id) VALUES($1,$2,$3,$4,$5)`, star, fmt.Sprintf("project-%d", in.ID), name, func() string {
					if kind == "station" {
						return "industry"
					}
					return "trade"
				}(), cid)
				if err != nil {
					return true, err
				}
			}
			if err = seedCommerce(ctx, tx); err != nil {
				return true, err
			}
			err = recordEvent(ctx, tx, u.ID, "project_complete", map[string]any{"project": in.ID, "star": star})
		}
	}
	return true, err
}
func (a *App) portalCommand(ctx context.Context, tx pgx.Tx, u User, s *Ship, g *Galaxy, in Command) error {
	if err := idle(ctx, tx, *s); err != nil {
		return err
	}
	var active bool
	var required, contributed int64
	if err := tx.QueryRow(ctx, `SELECT required,contributed,active FROM portal WHERE id=1 FOR UPDATE`).Scan(&required, &contributed, &active); err != nil {
		return err
	}
	if in.Action == "portal_travel" {
		if !active {
			return errors.New("Портал ещё не активирован")
		}
		near := false
		target := 0
		for _, id := range coreSystems(g) {
			if s.Star == id {
				near = true
			}
			if target == 0 {
				target = id
			}
		}
		if s.Star > 1000 {
			near = s.Star == 1001
		}
		if !near {
			return errors.New("Прибудьте в одну из систем у чёрной дыры")
		}
		if s.Star <= 1000 {
			target = 1001
		}
		_, err := tx.Exec(ctx, `UPDATE ships SET star=$2,object_id='entry',position_mode='approach',waypoints='[]',reserved_fuel=0 WHERE id=$1`, s.ID, target)
		return err
	}
	if active || in.Amount < 1 || in.Amount > required-contributed {
		return errors.New("Некорректный вклад")
	}
	if err := debit(ctx, tx, u.ID, in.Amount); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO portal_contributions(user_id,amount) VALUES($1,$2)`, u.ID, in.Amount); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE portal SET contributed=contributed+$1,active=(contributed+$1>=required) WHERE id=1`, in.Amount)
	if err != nil {
		return err
	}
	if contributed+in.Amount >= required {
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT data FROM galaxies WHERE name='Edem' FOR UPDATE`).Scan(&raw); err != nil {
			return err
		}
		var expanded Galaxy
		if err = json.Unmarshal(raw, &expanded); err != nil {
			return err
		}
		if len(expanded.Stars) == 1000 {
			other := generateGalaxy(1000, 20261027)
			for _, s := range other.Stars {
				s.ID += 1000
				s.Name = fmt.Sprintf("NX-%04d", s.ID-1000)
				s.Galaxy = "Edem-II"
				if s.Region < 0 {
					s.Region = -2
				} else {
					s.Region += 4
				}
				for i := range s.Planets {
					s.Planets[i].Name = fmt.Sprintf("%s-%c", s.Name, 'b'+i)
				}
				expanded.Stars = append(expanded.Stars, s)
			}
			expanded.Regions = append(expanded.Regions, frontierRegions...)
			for _, lane := range other.Lanes {
				lane.From += 1000
				lane.To += 1000
				expanded.Lanes = append(expanded.Lanes, lane)
			}
			expanded.NamingVersion = 0
			diversifySystemNames(&expanded)
			arrangeFrontierRings(&expanded)
			raw, _ = json.Marshal(expanded)
			if _, err = tx.Exec(ctx, `UPDATE galaxies SET data=$1 WHERE name='Edem'`, raw); err != nil {
				return err
			}
		}
		return recordEvent(ctx, tx, u.ID, "portal_open", map[string]any{"galaxy": "Edem-II"})
	}
	return nil
}
