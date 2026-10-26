package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Ship struct {
	Star         int            `json:"star"`
	Destination  *int           `json:"destination"`
	Arrival      *time.Time     `json:"arrival"`
	Fuel         int            `json:"fuel"`
	Cargo        map[string]int `json:"cargo"`
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	ObjectID     string         `json:"object_id"`
	PositionMode string         `json:"position_mode"`
	Departure    *time.Time     `json:"departure"`
	Path         []int          `json:"path"`
	LocalTarget  *string        `json:"local_target"`
	LocalMode    *string        `json:"local_mode"`
	LocalStarted *time.Time     `json:"local_started"`
	LocalArrival *time.Time     `json:"local_arrival"`
}
type Offer struct {
	Resource string `json:"resource"`
	Stock    int    `json:"stock"`
	Buy      int    `json:"buy"`
	Sell     int    `json:"sell"`
}

func marketStock(star Star) map[string]int {
	stock := map[string]int{}
	for _, r := range resources {
		stock[r.ID] = 30
	}
	for _, r := range regions {
		if r.ID == star.Region {
			stock[r.Specialty] = 100
		}
	}
	stock["agni"] = 100
	return stock
}
func offers(star Star, stock map[string]int) []Offer {
	out := []Offer{}
	for _, r := range resources {
		price := r.Price
		for _, reg := range regions {
			if reg.ID == star.Region && reg.Specialty == r.ID {
				price = price * 2 / 3
			}
		}
		price += star.ID % 5
		if stock[r.ID] < 10 {
			price += 5
		}
		out = append(out, Offer{r.ID, stock[r.ID], price + 2, price})
	}
	return out
}
func cargoSize(c map[string]int) int {
	n := 0
	for _, q := range c {
		n += q
	}
	return n
}
func ensureShip(ctx context.Context, tx pgx.Tx, id int64) (Ship, error) {
	var s Ship
	_, err := tx.Exec(ctx, `INSERT INTO ships(user_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
	if err != nil {
		return s, err
	}
	arrivalTag, err := tx.Exec(ctx, `UPDATE ships SET star=destination,destination=NULL,arrival=NULL,object_id='entry',position_mode='approach',departure=NULL,travel_path='[]' WHERE user_id=$1 AND arrival<=now()`, id)
	if err != nil {
		return s, err
	}
	localTag, localErr := tx.Exec(ctx, `UPDATE ships SET object_id=local_target,position_mode=local_mode,local_target=NULL,local_mode=NULL,local_started=NULL,local_arrival=NULL WHERE user_id=$1 AND local_arrival<=now()`, id)
	if localErr != nil {
		return s, localErr
	}
	var raw, path []byte
	err = tx.QueryRow(ctx, `SELECT star,destination,arrival,fuel,cargo,id,name,object_id,position_mode,departure,travel_path,local_target,local_mode,local_started,local_arrival FROM ships WHERE user_id=$1 FOR UPDATE`, id).Scan(&s.Star, &s.Destination, &s.Arrival, &s.Fuel, &raw, &s.ID, &s.Name, &s.ObjectID, &s.PositionMode, &s.Departure, &path, &s.LocalTarget, &s.LocalMode, &s.LocalStarted, &s.LocalArrival)
	if err == nil {
		err = json.Unmarshal(raw, &s.Cargo)
		if err == nil {
			err = json.Unmarshal(path, &s.Path)
		}
	}
	if err == nil && (arrivalTag.RowsAffected() > 0 || localTag.RowsAffected() > 0) {
		err = recordEvent(ctx, tx, id, "arrival", map[string]any{"star": s.Star, "object": s.ObjectID})
	}
	return s, err
}
func (a *App) expedition(w http.ResponseWriter, r *http.Request) {
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
	tx, err := a.db.Begin(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	// All mutations for one player serialize, including parallel browser requests.
	if err = tx.QueryRow(ctx, `SELECT credits FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&u.Credits); err != nil {
		a.internal(w, err)
		return
	}
	ship, err := ensureShip(ctx, tx, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	var raw []byte
	star := g.Stars[ship.Star-1]
	stock := marketStock(star)
	raw, _ = json.Marshal(stock)
	if _, err = tx.Exec(ctx, `INSERT INTO local_markets(star,stock) VALUES($1,$2) ON CONFLICT DO NOTHING`, ship.Star, raw); err != nil {
		a.internal(w, err)
		return
	}
	var wallet int64
	if err = tx.QueryRow(ctx, `SELECT credits,stock FROM local_markets WHERE star=$1 FOR UPDATE`, ship.Star).Scan(&wallet, &raw); err != nil {
		a.internal(w, err)
		return
	}
	if err = json.Unmarshal(raw, &stock); err != nil {
		a.internal(w, err)
		return
	}
	var job *MiningJob
	var active MiningJob
	err = tx.QueryRow(ctx, `SELECT resource,quantity,finish_tick,started_at,planet FROM mining_jobs WHERE user_id=$1`, u.ID).Scan(&active.Resource, &active.Quantity, &active.FinishTick, &active.StartedAt, &active.Planet)
	if err == nil {
		job = &active
	} else if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	err = nil
	var worldTick int64
	if err = tx.QueryRow(ctx, `SELECT tick FROM world WHERE id=1`).Scan(&worldTick); err != nil {
		a.internal(w, err)
		return
	}
	reservedCargo := 0
	if job != nil {
		reservedCargo = job.Quantity
	}
	var surveyed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM surveys WHERE user_id=$1 AND star=$2)`, u.ID, ship.Star).Scan(&surveyed); err != nil {
		a.internal(w, err)
		return
	}
	if r.Method == "POST" {
		var in struct {
			Action   string `json:"action"`
			To       int    `json:"to"`
			Resource string `json:"resource"`
			Quantity int    `json:"quantity"`
			Planet   int    `json:"planet"`
			Target   string `json:"target"`
			Mode     string `json:"mode"`
		}
		if !decode(w, r, &in) {
			return
		}
		problem := ""
		if ship.Destination != nil || ship.LocalTarget != nil {
			problem = "Корабль в пути"
		} else {
			switch in.Action {
			case "move":
				if job != nil {
					problem = "Дождитесь завершения добычи"
					break
				}
				var target *Site
				for _, site := range systemSites(star) {
					if site.ID == in.Target {
						copy := site
						target = &copy
					}
				}
				if target == nil {
					problem = "Объект не найден"
					break
				}
				if in.Mode != "approach" && in.Mode != "orbit" && in.Mode != "docked" {
					problem = "Некорректный манёвр"
					break
				}
				if in.Mode == "docked" && target.Kind != "Орбитальная станция" || in.Mode == "orbit" && target.Details == nil {
					problem = "Манёвр недоступен для объекта"
					break
				}
				if ship.Fuel < 1 {
					problem = "Недостаточно топлива"
					break
				}
				if nearObject(ship, target.ID) && ship.PositionMode == in.Mode {
					problem = "Корабль уже на месте"
					break
				}
				duration := 5 * time.Second
				now := time.Now()
				arrival := now.Add(duration)
				ship.LocalTarget = &in.Target
				ship.LocalMode = &in.Mode
				ship.LocalStarted = &now
				ship.LocalArrival = &arrival
				ship.Fuel--
			case "cancel_mine":
				if _, err = tx.Exec(ctx, `SELECT resource FROM resource_quotas WHERE star=$1 AND object=$2 AND resource='*' FOR UPDATE`, ship.Star, func() int {
					if job != nil {
						return job.Planet
					}
					return 0
				}()); err != nil {
					break
				}
				if job == nil {
					problem = "Нет активной добычи"
					break
				}
				if _, err = tx.Exec(ctx, `UPDATE resource_quotas q SET used=GREATEST(used-j.quantity,0) FROM mining_jobs j WHERE j.user_id=$1 AND q.star=j.star AND q.object=j.planet AND q.resource IN ('*',j.resource) AND q.window_start=j.quota_start`, u.ID); err == nil {
					_, err = tx.Exec(ctx, `DELETE FROM mining_jobs WHERE user_id=$1`, u.ID)
				}
				job = nil
			case "scan":
				if !surveyed {
					if u.Credits < 10 {
						problem = "Недостаточно кредитов"
					} else {
						u.Credits -= 10
						if _, err = tx.Exec(ctx, `UPDATE users SET xp=xp+10 WHERE id=$1`, u.ID); err != nil {
							break
						}
						_, err = tx.Exec(ctx, `INSERT INTO surveys(user_id,star) VALUES($1,$2)`, u.ID, ship.Star)
						surveyed = true
					}
				}
			case "travel":
				if job != nil {
					problem = "Дождитесь завершения добычи"
					break
				}
				path := g.path(ship.Star, in.To)
				if len(path) < 2 {
					problem = "Нет допустимого маршрута"
				} else if ship.Fuel < (len(path)-1)*2 {
					problem = "Недостаточно топлива"
				} else {
					ship.Fuel -= (len(path) - 1) * 2
					arrival := time.Now().Add(time.Duration(len(path)-1) * 5 * time.Second)
					ship.Arrival = &arrival
					now := time.Now()
					ship.Departure = &now
					ship.Path = path
					ship.Destination = &in.To
				}
			case "refuel":
				if ship.Fuel >= 40 {
					problem = "Бак полон"
				} else if ship.Cargo["agni"] < 1 {
					problem = "Нужен Агни в трюме"
				} else {
					ship.Cargo["agni"]--
					ship.Fuel += 5
					if ship.Fuel > 40 {
						ship.Fuel = 40
					}
				}
			case "buy", "sell":
				if !hasStation(ship.Star) || !docked(ship) {
					problem = "Для торговли требуется стыковка с портом"
					break
				}
				if _, exists := resource(in.Resource); !exists || in.Quantity < 1 || in.Quantity > 60 {
					problem = "Некорректная сделка"
					break
				}
				var offer Offer
				for _, o := range offers(star, stock) {
					if o.Resource == in.Resource {
						offer = o
					}
				}
				q := in.Quantity
				if in.Action == "buy" {
					cost := int64(q * offer.Buy)
					if stock[in.Resource] < q {
						problem = "Нет товара на рынке"
					} else if cargoSize(ship.Cargo)+reservedCargo+q > 60 {
						problem = "Трюм заполнен"
					} else if u.Credits < cost {
						problem = "Недостаточно кредитов"
					} else {
						stock[in.Resource] -= q
						ship.Cargo[in.Resource] += q
						u.Credits -= cost
						wallet += cost
					}
				}
				if in.Action == "sell" {
					cost := int64(q * offer.Sell)
					if ship.Cargo[in.Resource] < q {
						problem = "Нет груза для продажи"
					} else if wallet < cost {
						problem = "У рынка недостаточно кредитов"
					} else {
						ship.Cargo[in.Resource] -= q
						stock[in.Resource] += q
						u.Credits += cost
						wallet -= cost
					}
				}
			case "mine":
				if job != nil {
					problem = "Добыча уже выполняется"
					break
				}
				if !surveyed {
					problem = "Сначала исследуйте систему"
					break
				}
				found := false
				var sourceID string
				for _, site := range systemSites(star) {
					if site.Planet == in.Planet {
						sourceID = site.ID
						for _, id := range site.Resources {
							if id == in.Resource {
								found = true
							}
						}
					}
				}
				if !found {
					problem = "Ресурс отсутствует на планете"
					break
				}
				if !nearObject(ship, sourceID) {
					problem = "Приблизьтесь к объекту добычи"
					break
				}
				if cargoSize(ship.Cargo) >= 60 {
					problem = "Трюм заполнен"
					break
				}
				count, start, extractErr := reserveExtraction(ctx, tx, ship.Star, in.Planet, in.Resource)
				err = extractErr
				if err != nil {
					break
				}
				if cargoSize(ship.Cargo)+count > 60 {
					problem = "Трюм заполнен"
					break
				}
				if count == 0 {
					problem = "Лимит добычи исчерпан"
					break
				}
				_, err = tx.Exec(ctx, `INSERT INTO mining_jobs(user_id,star,planet,resource,quantity,finish_tick,quota_start) VALUES($1,$2,$3,$4,$5,$6,$7)`, u.ID, ship.Star, in.Planet, in.Resource, count, worldTick+1, start)
				if err == nil {
					job = &MiningJob{Resource: in.Resource, Quantity: count, FinishTick: worldTick + 1, StartedAt: time.Now(), Planet: in.Planet}
				}

			default:
				problem = "Неизвестное действие"
			}
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if problem != "" {
			fail(w, 409, problem)
			return
		}
		stockJSON, _ := json.Marshal(stock)
		if _, err = tx.Exec(ctx, `UPDATE users SET credits=$2 WHERE id=$1`, u.ID, u.Credits); err != nil {
			a.internal(w, err)
			return
		}
		if err = saveShip(ctx, tx, u.ID, ship); err != nil {
			a.internal(w, err)
			return
		}
		if err = recordEvent(ctx, tx, u.ID, in.Action, map[string]any{"star": ship.Star, "resource": in.Resource, "quantity": in.Quantity, "object": in.Target, "destination": in.To}); err != nil {
			a.internal(w, err)
			return
		}

		if _, err = tx.Exec(ctx, `UPDATE local_markets SET credits=$2,stock=$3 WHERE star=$1`, ship.Star, wallet, stockJSON); err != nil {
			a.internal(w, err)
			return
		}
	}
	sites := systemSites(star)
	if surveyed {
		sites, err = quotaView(ctx, tx, star, sites)
		if err != nil {
			a.internal(w, err)
			return
		}
	} else {
		for i := range sites {
			sites[i].Resources = nil
			sites[i].Details = nil
		}
	}
	if !hasStation(ship.Star) {
		stock = map[string]int{}
	}
	if !surveyed {
		star.Planets = nil
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	marketOffers := offers(star, stock)
	if !hasStation(ship.Star) {
		marketOffers = []Offer{}
	}
	reply(w, 200, map[string]any{"server_time": time.Now(), "job": job, "world_tick": worldTick, "station": hasStation(ship.Star), "docked": docked(ship), "sites": sites, "ship": ship, "system": star, "region": regionName(star.Region), "surveyed": surveyed, "credits": u.Credits, "resources": resourceCatalog(), "offers": marketOffers, "market_credits": wallet, "capacity": 60})
}

// The legacy prototype ore mine is separate. Bots trade only finite cargo against
// finite station wallets; neither this routine nor a trade creates currency.
func tradeBots(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE is_bot ORDER BY id`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i, id := range ids {
		var credits int64
		if err = tx.QueryRow(ctx, `SELECT credits FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&credits); err != nil {
			return err
		}
		ship, err := ensureShip(ctx, tx, id)
		if err != nil {
			return err
		}
		star := Star{ID: 1, Region: 0}
		stock := marketStock(star)
		raw, _ := json.Marshal(stock)
		if _, err = tx.Exec(ctx, `INSERT INTO local_markets(star,stock) VALUES(1,$1) ON CONFLICT DO NOTHING`, raw); err != nil {
			return err
		}
		var wallet int64
		if err = tx.QueryRow(ctx, `SELECT credits,stock FROM local_markets WHERE star=1 FOR UPDATE`).Scan(&wallet, &raw); err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &stock); err != nil {
			return err
		}
		res := resources[i%len(resources)].ID
		var offer Offer
		for _, o := range offers(star, stock) {
			if o.Resource == res {
				offer = o
			}
		}
		if ship.Cargo[res] > 0 && wallet >= int64(offer.Sell) {
			ship.Cargo[res]--
			stock[res]++
			credits += int64(offer.Sell)
			wallet -= int64(offer.Sell)
		} else if ship.Cargo[res] == 0 && stock[res] > 0 && credits >= int64(offer.Buy) && cargoSize(ship.Cargo) < 60 {
			ship.Cargo[res]++
			stock[res]--
			credits -= int64(offer.Buy)
			wallet += int64(offer.Buy)
		}
		cargo, _ := json.Marshal(ship.Cargo)
		raw, _ = json.Marshal(stock)
		for _, q := range []struct {
			sql  string
			args []any
		}{{`UPDATE ships SET cargo=$2 WHERE user_id=$1`, []any{id, cargo}}, {`UPDATE users SET credits=$2 WHERE id=$1`, []any{id, credits}}, {`UPDATE local_markets SET credits=$1,stock=$2 WHERE star=1`, []any{wallet, raw}}} {
			if _, err = tx.Exec(ctx, q.sql, q.args...); err != nil {
				return fmt.Errorf("bot trade: %w", err)
			}
		}
	}
	return nil
}
