package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"hash/fnv"
	"sort"
	"strconv"
)

type TradeProfile struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Goods []string `json:"goods"`
}

var tradeProfiles = []TradeProfile{
	{"trade", "Торговый узел", []string{"iron", "manna", "water", "agni", "carbon", "silicon", "nectar", "electrum"}},
	{"metals", "Металлургия", []string{"iron", "orichalcum", "adamant", "carbon"}},
	{"materials", "Материалы", []string{"silicon", "carbon", "vajra", "adamant", "orichalcum"}},
	{"industry", "Промышленность", []string{"iron", "silicon", "adamant", "electrum", "vajra", "carbon"}},
	{"food", "Продукты", []string{"manna", "nectar", "ambrosia", "water"}},
	{"agro", "Агрокомплекс", []string{"manna", "soma", "ambrosia", "water", "carbon"}},
	{"chemistry", "Химический комплекс", []string{"azoth", "carbon", "water", "soma", "silicon"}},
	{"energy", "Энергетика", []string{"agni", "azoth", "carbon", "water"}},
}

func catalogueHash(star int, id string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strconv.Itoa(star) + "/" + id))
	return h.Sum32()
}
func stationProfile(star int, id string) string {
	if (star == 1 || star == 5) && id == "station" {
		return "trade"
	}
	return tradeProfiles[int(catalogueHash(star, id))%len(tradeProfiles)].ID
}
func stationGoods(star int, id, spec string) []string {
	var pool []string
	for _, p := range tradeProfiles {
		if p.ID == spec {
			pool = p.Goods
		}
	}
	if len(pool) == 0 {
		pool = tradeProfiles[0].Goods
	}
	pool = append([]string(nil), pool...)
	sort.Slice(pool, func(i, j int) bool { return catalogueHash(star, id+pool[i]) < catalogueHash(star, id+pool[j]) })
	n := 2 + int(catalogueHash(star, id+"size"))%(len(pool)-2)
	goods := append([]string(nil), pool[:n]...)
	if (star == 1 || star == 5) && id == "station" {
		goods = []string{"iron", "manna", "water", "agni", "nectar"}
	}
	return goods
}
func stockForStation(star Star, goods []string) map[string]int {
	all := marketStock(star)
	out := map[string]int{}
	for _, id := range goods {
		out[id] = all[id]
	}
	return out
}

// Executed once for each station, including stations constructed by players.
// Legacy balances/stock are retained; only the advertised catalogue is restricted.
func seedCommerce(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT star,object_id,specialization,company_id,shipyard,ship_stock,yard_seeded,goods IS NULL FROM stations WHERE goods IS NULL OR (shipyard AND NOT yard_seeded) ORDER BY star,object_id`)
	if err != nil {
		return err
	}
	type port struct {
		star            int
		id, spec        string
		company         *int64
		yard            bool
		stock           int
		seeded, missing bool
	}
	var list []port
	for rows.Next() {
		var p port
		if err = rows.Scan(&p.star, &p.id, &p.spec, &p.company, &p.yard, &p.stock, &p.seeded, &p.missing); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, p := range list {
		if p.missing {
			if p.company == nil {
				p.spec = stationProfile(p.star, p.id)
			} else if p.spec == "general" {
				p.spec = "trade"
			} else if p.spec == "refinery" {
				p.spec = "industry"
			}
			batch.Queue(`UPDATE stations SET specialization=$3,goods=$4 WHERE star=$1 AND object_id=$2`, p.star, p.id, p.spec, stationGoods(p.star, p.id, p.spec))
		}
		if p.yard && !p.seeded {
			models := map[string]bool{"pilgrim": true}
			h := int(catalogueHash(p.star, p.id))
			models[shipClasses[1+h%4].ID] = true
			models[shipClasses[1+(h/7)%4].ID] = true
			if p.star == 1 && p.id == "station" {
				models = map[string]bool{"pilgrim": true, "wanderer": true}
			}
			if p.star == 5 && p.id == "station" {
				models = map[string]bool{"pilgrim": true, "forge": true}
			}
			for _, c := range shipClasses {
				if !models[c.ID] {
					continue
				}
				q := 1 + (h+len(c.ID))%3
				price := c.Price * int64(95+h%11) / 100
				if c.ID == "pilgrim" {
					q = p.stock
					price = c.Price
				}
				batch.Queue(`INSERT INTO shipyard_stock(star,station,ship_class,quantity,price) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, p.star, p.id, c.ID, q, price)
			}
			batch.Queue(`UPDATE stations SET yard_seeded=TRUE WHERE star=$1 AND object_id=$2`, p.star, p.id)
		}
	}
	if batch.Len() == 0 {
		return nil
	}
	return tx.SendBatch(ctx, batch).Close()
}
func marketAt(ctx context.Context, tx pgx.Tx, star Star, id string) (map[string]int, int64, error) {
	var goods []string
	err := tx.QueryRow(ctx, `SELECT goods FROM stations WHERE star=$1 AND object_id=$2`, star.ID, id).Scan(&goods)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]int{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	stock := stockForStation(star, goods)
	raw, _ := json.Marshal(stock)
	if _, err = tx.Exec(ctx, `INSERT INTO local_markets(star,station,stock) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, star.ID, id, raw); err != nil {
		return nil, 0, err
	}
	var wallet int64
	if err = tx.QueryRow(ctx, `SELECT stock,credits FROM local_markets WHERE star=$1 AND station=$2 FOR UPDATE`, star.ID, id).Scan(&raw, &wallet); err != nil {
		return nil, 0, err
	}
	stock = map[string]int{}
	err = json.Unmarshal(raw, &stock)
	return stock, wallet, err
}
func (a *App) globalPorts(ctx context.Context, uid int64, g *Galaxy) ([]map[string]any, error) {
	rows, err := a.db.Query(ctx, `SELECT s.star,s.object_id,s.name,s.specialization,s.goods,m.stock,COALESCE(m.credits,5000),CASE WHEN c.founder_key='founder-02' AND COALESCE(r.points,0)<20 THEN 2 ELSE 0 END FROM stations s LEFT JOIN local_markets m ON m.star=s.star AND m.station=s.object_id LEFT JOIN companies c ON c.id=s.founder LEFT JOIN reputation r ON r.company_id=s.founder AND r.user_id=$1 ORDER BY s.star,s.object_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var star, fee int
		var id, name, spec string
		var goods []string
		var raw []byte
		var credits int64
		if err = rows.Scan(&star, &id, &name, &spec, &goods, &raw, &credits, &fee); err != nil {
			return nil, err
		}
		if star < 1 || star > len(g.Stars) {
			continue
		}
		stock := stockForStation(g.Stars[star-1], goods)
		if raw != nil {
			stock = map[string]int{}
			if err = json.Unmarshal(raw, &stock); err != nil {
				return nil, err
			}
		}
		list := stationOffers(g.Stars[star-1], stock, goods)
		for i := range list {
			list[i] = feeOffer(list[i], fee)
		}
		out = append(out, map[string]any{"star": star, "station": id, "name": name, "specialization": spec, "credits": credits, "fee_percent": fee, "offers": list})
	}
	return out, rows.Err()
}
