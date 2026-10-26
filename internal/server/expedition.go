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
	Star        int            `json:"star"`
	Destination *int           `json:"destination"`
	Arrival     *time.Time     `json:"arrival"`
	Fuel        int            `json:"fuel"`
	Cargo       map[string]int `json:"cargo"`
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
	_, err = tx.Exec(ctx, `UPDATE ships SET star=destination,destination=NULL,arrival=NULL WHERE user_id=$1 AND arrival<=now()`, id)
	if err != nil {
		return s, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT star,destination,arrival,fuel,cargo FROM ships WHERE user_id=$1 FOR UPDATE`, id).Scan(&s.Star, &s.Destination, &s.Arrival, &s.Fuel, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &s.Cargo)
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
		}
		if !decode(w, r, &in) {
			return
		}
		problem := ""
		if ship.Destination != nil {
			problem = "Корабль в пути"
		} else {
			switch in.Action {
			case "scan":
				if !surveyed {
					if u.Credits < 10 {
						problem = "Недостаточно кредитов"
					} else {
						u.Credits -= 10
						_, err = tx.Exec(ctx, `INSERT INTO surveys(user_id,star) VALUES($1,$2)`, u.ID, ship.Star)
						surveyed = true
					}
				}
			case "travel":
				path := g.path(ship.Star, in.To)
				if len(path) < 2 {
					problem = "Нет допустимого маршрута"
				} else if ship.Fuel < (len(path)-1)*2 {
					problem = "Недостаточно топлива"
				} else {
					ship.Fuel -= (len(path) - 1) * 2
					arrival := time.Now().Add(time.Duration(len(path)-1) * 5 * time.Second)
					ship.Arrival = &arrival
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
					} else if cargoSize(ship.Cargo)+q > 60 {
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
				if !surveyed {
					problem = "Сначала исследуйте систему"
					break
				}
				if in.Planet < 1 || in.Planet > len(star.Planets) {
					problem = "Некорректная планета"
					break
				}
				found := false
				for _, id := range star.Planets[in.Planet-1].Resources {
					if id == in.Resource {
						found = true
					}
				}
				if !found {
					problem = "Ресурс отсутствует на планете"
					break
				}
				if cargoSize(ship.Cargo)+3 > 60 {
					problem = "Трюм заполнен"
					break
				}
				var ready bool
				err = tx.QueryRow(ctx, `SELECT last_mine<=now()-interval '10 seconds' FROM ships WHERE user_id=$1`, u.ID).Scan(&ready)
				if err != nil {
					break
				}
				if !ready {
					problem = "Добыча доступна раз в 10 секунд"
					break
				}
				_, err = tx.Exec(ctx, `INSERT INTO deposits(star,planet,resource) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, ship.Star, in.Planet, in.Resource)
				if err != nil {
					break
				}
				var left int
				err = tx.QueryRow(ctx, `UPDATE deposits SET remaining=remaining-3 WHERE star=$1 AND planet=$2 AND resource=$3 AND remaining>=3 RETURNING remaining`, ship.Star, in.Planet, in.Resource).Scan(&left)
				if errors.Is(err, pgx.ErrNoRows) {
					err = nil
					problem = "Месторождение истощено"
					break
				}
				if err == nil {
					ship.Cargo[in.Resource] += 3
					_, err = tx.Exec(ctx, `UPDATE ships SET last_mine=now() WHERE user_id=$1`, u.ID)
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
		cargo, _ := json.Marshal(ship.Cargo)
		stockJSON, _ := json.Marshal(stock)
		if _, err = tx.Exec(ctx, `UPDATE users SET credits=$2 WHERE id=$1`, u.ID, u.Credits); err != nil {
			a.internal(w, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE ships SET destination=$2,arrival=$3,fuel=$4,cargo=$5 WHERE user_id=$1`, u.ID, ship.Destination, ship.Arrival, ship.Fuel, cargo); err != nil {
			a.internal(w, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE local_markets SET credits=$2,stock=$3 WHERE star=$1`, ship.Star, wallet, stockJSON); err != nil {
			a.internal(w, err)
			return
		}
	}
	if !surveyed {
		star.Planets = nil
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]any{"ship": ship, "system": star, "region": regionName(star.Region), "surveyed": surveyed, "credits": u.Credits, "resources": resources, "offers": offers(star, stock), "market_credits": wallet, "capacity": 60})
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
