package tradecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/universe"
)

type TradeProfile struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Goods []string `json:"goods"`
}

var TradeProfiles = []TradeProfile{
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

func StationProfile(star int, id string) string {
	if (star == 1 || star == 5) && id == "station" {
		return "trade"
	}
	return TradeProfiles[int(catalogueHash(star, id))%len(TradeProfiles)].ID
}

func stationGoods(star int, id, spec string) []string {
	var pool []string
	for _, p := range TradeProfiles {
		if p.ID == spec {
			pool = p.Goods
		}
	}
	if len(pool) == 0 {
		pool = TradeProfiles[0].Goods
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

func StockForStation(star universe.Star, goods []string) map[string]int {
	all := marketStock(star)
	out := map[string]int{}
	for _, id := range goods {
		out[id] = all[id]
	}
	return out
}

// Executed once for each station, including stations constructed by players.
// Legacy balances/stock are retained; only the advertised catalogue is restricted.
func SeedCommerce(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT star,object_id,specialization,company_id FROM stations WHERE goods IS NULL AND active ORDER BY star,object_id`)
	if err != nil {
		return err
	}
	type port struct {
		star     int
		id, spec string
		company  *int64
	}
	var list []port
	for rows.Next() {
		var p port
		if err = rows.Scan(&p.star, &p.id, &p.spec, &p.company); err != nil {
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
		if p.company == nil {
			p.spec = StationProfile(p.star, p.id)
		} else if p.spec == "general" {
			p.spec = "trade"
		} else if p.spec == "refinery" {
			p.spec = "industry"
		}
		batch.Queue(`UPDATE stations SET specialization=$3,goods=$4 WHERE star=$1 AND object_id=$2 AND active`, p.star, p.id, p.spec, stationGoods(p.star, p.id, p.spec))
	}
	if batch.Len() > 0 {
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return err
		}
	}
	return seedShipyards(ctx, tx)
}

func MarketAt(ctx context.Context, tx pgx.Tx, star universe.Star, id string) (map[string]int, int64, error) {
	var goods []string
	err := tx.QueryRow(ctx, `SELECT goods FROM stations WHERE star=$1 AND object_id=$2 AND active`, star.ID, id).Scan(&goods)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]int{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	stock := StockForStation(star, goods)
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
