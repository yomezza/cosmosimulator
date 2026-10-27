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
	Class        string         `json:"class"`
	Spec         ShipClass      `json:"spec"`
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
	Extractor    string         `json:"extractor"`
	ReservedFuel int            `json:"reserved_fuel"`
	Active       bool           `json:"active"`
	Waypoints    []int          `json:"waypoints"`
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
	for _, r := range allRegions() {
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
		for _, reg := range allRegions() {
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
func stationOffers(star Star, stock map[string]int, goods []string) []Offer {
	allowed := map[string]bool{}
	for _, id := range goods {
		allowed[id] = true
	}
	out := []Offer{}
	for _, o := range offers(star, stock) {
		if allowed[o.Resource] {
			out = append(out, o)
		}
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
	if _, err := tx.Exec(ctx, `INSERT INTO ships(user_id,active) VALUES($1,TRUE) ON CONFLICT DO NOTHING`, id); err != nil {
		return Ship{}, err
	}
	if err := settleShips(ctx, tx, id); err != nil {
		return Ship{}, err
	}
	var sid int64
	if err := tx.QueryRow(ctx, `SELECT id FROM ships WHERE user_id=$1 AND active`, id).Scan(&sid); err != nil {
		return Ship{}, err
	}
	return readShip(ctx, tx, sid)
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
	if r.Method == "POST" {
		if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(784218)`); err != nil {
			a.internal(w, err)
			return
		}
		g, err = a.loadUniverse(ctx)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
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
	star := g.Stars[ship.Star-1]
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784219,$1)`, ship.Star); err != nil {
		a.internal(w, err)
		return
	}
	marketID := ship.ObjectID
	if !docked(ship) {
		marketID = ""
	}
	stock, wallet, err := marketAt(ctx, tx, star, marketID)
	if err != nil {
		a.internal(w, err)
		return
	}

	var job *MiningJob
	var active MiningJob
	err = tx.QueryRow(ctx, `SELECT resource,quantity,finish_tick,started_at,planet FROM mining_jobs WHERE ship_id=$1`, ship.ID).Scan(&active.Resource, &active.Quantity, &active.FinishTick, &active.StartedAt, &active.Planet)
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
			Ship          int64  `json:"ship"`
			Action        string `json:"action"`
			To            int    `json:"to"`
			Resource      string `json:"resource"`
			Quantity      int    `json:"quantity"`
			Planet        int    `json:"planet"`
			Target        string `json:"target"`
			Mode          string `json:"mode"`
			ExpectedPrice *int   `json:"expected_price"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Ship > 0 && in.Ship != ship.ID {
			fail(w, 409, "Активный корабль изменился. Повторите действие")
			return
		}
		if err = reason(ctx, tx, in.Action); err != nil {
			a.internal(w, err)
			return
		}
		problem := ""
		if ship.Destination != nil || ship.LocalTarget != nil {
			problem = "Корабль в пути"
		} else {
			switch in.Action {
			case "move":
				if researchBusy(ctx, tx, ship.ID) {
					problem = "Сначала приостановите исследование"
					break
				}
				if job != nil {
					problem = "Дождитесь завершения добычи"
					break
				}
				var target *Site
				sitesForMove, e := addCustomSites(ctx, tx, star, systemSites(star))
				if e != nil {
					err = e
					break
				}
				for _, site := range sitesForMove {
					if site.ID == in.Target {
						copy := site
						target = &copy
					}
				}
				visible, e := revealedSites(ctx, tx, u.ID, star.ID, sitesForMove, surveyed)
				if e != nil {
					err = e
					break
				}
				knownTarget := false
				// Public port coordinates are broadcast by the market. Docking
				// does not disclose the system's unscanned planets or deposits.
				if target != nil && target.Kind == "Орбитальная станция" {
					knownTarget = true
				}
				for _, site := range visible {
					if site.ID == in.Target {
						knownTarget = true
					}
				}
				if !knownTarget {
					target = nil
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
				if ship.Fuel-ship.ReservedFuel < 1 {
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
				if _, err = tx.Exec(ctx, `UPDATE resource_quotas q SET used=GREATEST(used-j.quantity,0) FROM mining_jobs j WHERE j.ship_id=$1 AND q.star=j.star AND q.object=j.planet AND q.resource IN ('*',j.resource) AND q.window_start=j.quota_start`, ship.ID); err == nil {
					_, err = tx.Exec(ctx, `DELETE FROM mining_jobs WHERE ship_id=$1`, ship.ID)
				}
				job = nil
			case "scan":
				err = startResearch(ctx, tx, u.ID, &ship, star, g)
				if err != nil {
					problem = err.Error()
					err = nil
				} else {
					err = tx.QueryRow(ctx, `SELECT credits FROM users WHERE id=$1`, u.ID).Scan(&u.Credits)
				}
			case "travel":
				if researchBusy(ctx, tx, ship.ID) {
					problem = "Сначала приостановите исследование"
					break
				}
				if job != nil {
					problem = "Дождитесь завершения добычи"
					break
				}
				path := g.path(ship.Star, in.To)
				if len(path) < 2 {
					problem = "Нет допустимого маршрута"
				} else if ship.Fuel-ship.ReservedFuel < (len(path)-1)*ship.hull().JumpFuel {
					problem = "Недостаточно топлива"
				} else {
					ship.Fuel -= (len(path) - 1) * ship.hull().JumpFuel
					arrival := time.Now().Add(time.Duration((len(path)-1)*ship.hull().JumpSeconds) * time.Second)
					ship.Arrival = &arrival
					now := time.Now()
					ship.Departure = &now
					ship.Path = path
					ship.Destination = &in.To
				}
			case "refuel":
				if ship.Fuel >= ship.hull().Fuel {
					problem = "Бак полон"
				} else if ship.Cargo["agni"] < 1 {
					problem = "Нужен Агни в трюме"
				} else {
					ship.Cargo["agni"]--
					ship.Fuel += 5
					if ship.Fuel > ship.hull().Fuel {
						ship.Fuel = ship.hull().Fuel
					}
				}
			case "buy", "sell":
				if !stationAllows(ctx, tx, ship.Star, ship.ObjectID, in.Resource) {
					problem = "Станция не торгует этой категорией"
					break
				}
				if !docked(ship) {
					problem = "Для торговли требуется стыковка с портом"
					break
				}
				if _, exists := resource(in.Resource); !exists || in.Quantity < 1 || in.Quantity > ship.capacity() {
					problem = "Некорректная сделка"
					break
				}
				var offer Offer
				for _, o := range offers(star, stock) {
					if o.Resource == in.Resource {
						offer = feeOffer(o, portFee(ctx, tx, u.ID, ship.Star, ship.ObjectID))
					}
				}
				if in.ExpectedPrice != nil {
					price := offer.Buy
					if in.Action == "sell" {
						price = offer.Sell
					}
					if *in.ExpectedPrice != price {
						problem = "Цена изменилась. Проверьте предложение ещё раз"
						break
					}
				}
				q := in.Quantity
				if in.Action == "buy" {
					cost := int64(q * offer.Buy)
					if stock[in.Resource] < q {
						problem = "Нет товара на рынке"
					} else if cargoSize(ship.Cargo)+reservedCargo+q > ship.capacity() {
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
				if problem == "" {
					_, err = tx.Exec(ctx, `INSERT INTO reputation(user_id,company_id,points) SELECT $1,founder,1 FROM stations WHERE star=$2 AND object_id=$3 AND founder IS NOT NULL ON CONFLICT(user_id,company_id) DO UPDATE SET points=reputation.points+1`, u.ID, ship.Star, ship.ObjectID)
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
				if cargoSize(ship.Cargo) >= ship.capacity() {
					problem = "Трюм заполнен"
					break
				}
				if !extractorAccepts(ship.Extractor, in.Resource) {
					problem = "Установите подходящий добытчик"
					break
				}
				count, start, extractErr := reserveExtraction(ctx, tx, ship.Star, in.Planet, in.Resource, extractorYield(ship.Extractor, ship.Star, in.Planet)+ship.hull().MiningBonus)
				err = extractErr
				if err != nil {
					break
				}
				if cargoSize(ship.Cargo)+count > ship.capacity() {
					problem = "Трюм заполнен"
					break
				}
				if count == 0 {
					problem = "Лимит добычи исчерпан"
					break
				}
				_, err = tx.Exec(ctx, `INSERT INTO mining_jobs(user_id,star,planet,resource,quantity,finish_tick,quota_start,ship_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, u.ID, ship.Star, in.Planet, in.Resource, count, worldTick+1, start, ship.ID)
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

		if _, err = tx.Exec(ctx, `UPDATE local_markets SET credits=$2,stock=$3 WHERE star=$1 AND station=$4`, ship.Star, wallet, stockJSON, marketID); err != nil {
			a.internal(w, err)
			return
		}
	}
	sites, siteErr := addCustomSites(ctx, tx, star, systemSites(star))
	if siteErr != nil {
		a.internal(w, siteErr)
		return
	}
	if surveyed {
		sites, err = quotaView(ctx, tx, star, sites)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	sites, err = revealedSites(ctx, tx, u.ID, star.ID, sites, surveyed)
	if err != nil {
		a.internal(w, err)
		return
	}
	if !surveyed {
		star.Planets = nil
	}
	star.ResourceWeights = nil
	star.Ports = nil
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	marketOffers := []Offer{}
	fee := portFee(ctx, a.db, u.ID, ship.Star, marketID)
	var goods []string
	if docked(ship) {
		if e := a.db.QueryRow(ctx, `SELECT goods FROM stations WHERE star=$1 AND object_id=$2`, ship.Star, marketID).Scan(&goods); e != nil {
			a.internal(w, e)
			return
		}
	}
	for _, o := range stationOffers(star, stock, goods) {
		marketOffers = append(marketOffers, feeOffer(o, fee))
	}
	stationPresent := false
	for _, site := range sites {
		if site.Kind == "Орбитальная станция" {
			stationPresent = true
			break
		}
	}

	reply(w, 200, map[string]any{"server_time": time.Now(), "job": job, "world_tick": worldTick, "station": stationPresent, "docked": docked(ship), "sites": sites, "ship": ship, "system": star, "region": regionName(star.Region), "surveyed": surveyed, "credits": u.Credits, "resources": resourceCatalog(), "offers": marketOffers, "market_credits": wallet, "capacity": ship.capacity(), "research": researchView(ctx, a.db, u.ID)})
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
		stock, wallet, err := marketAt(ctx, tx, star, "station")
		if err != nil {
			return err
		}
		var raw []byte
		var goods []string
		if err = tx.QueryRow(ctx, `SELECT goods FROM stations WHERE star=1 AND object_id='station'`).Scan(&goods); err != nil {
			return err
		}
		available := stationOffers(star, stock, goods)
		if len(available) == 0 {
			continue
		}
		res := available[i%len(available)].Resource

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
		} else if ship.Cargo[res] == 0 && stock[res] > 0 && credits >= int64(offer.Buy) && cargoSize(ship.Cargo) < ship.capacity() {
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
		}{{`UPDATE ships SET cargo=$2 WHERE user_id=$1 AND active`, []any{id, cargo}}, {`UPDATE users SET credits=$2 WHERE id=$1`, []any{id, credits}}, {`UPDATE local_markets SET credits=$1,stock=$2 WHERE star=1 AND station='station'`, []any{wallet, raw}}} {
			if _, err = tx.Exec(ctx, q.sql, q.args...); err != nil {
				return fmt.Errorf("bot trade: %w", err)
			}
		}
	}
	return nil
}
