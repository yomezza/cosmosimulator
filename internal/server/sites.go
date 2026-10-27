package server

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

var originalCore = func() map[int]bool {
	g := generateGalaxy(1000, 20261026)
	m := map[int]bool{}
	for _, id := range coreSystems(&g) {
		m[id] = true
	}
	return m
}()

func addCustomSites(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, s Star, sites []Site) ([]Site, error) {
	rows, err := db.Query(ctx, `SELECT object_id,name FROM stations WHERE star=$1 AND active ORDER BY object_id`, s.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for _, v := range sites {
		seen[v.ID] = true
	}
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if !seen[id] {
			sites = append(sites, Site{ID: id, Name: name, Kind: "Орбитальная станция", Orbit: 2.1 + float64(len(sites))*.15})
		}
	}
	return sites, rows.Err()
}
func stationAllows(ctx context.Context, q querier, star int, id, res string) bool {
	var goods []string
	if err := q.QueryRow(ctx, `SELECT goods FROM stations WHERE star=$1 AND object_id=$2 AND active`, star, id).Scan(&goods); err != nil {
		return false
	}
	for _, item := range goods {
		if item == res {
			return true
		}
	}
	return false
}
func concentration(star, object int) int { return 35 + (star*17+object*11)%61 }
func quality(star, object int) string    { return fmt.Sprintf("Q%d", 1+(star*13+object*7)%5) }

// Port fees are included in the quote. Market wallets receive the same amount
// the player pays; no currency is created by the reputation discount.
func portFee(ctx context.Context, q querier, uid int64, star int, id string) int {
	var fee int
	_ = q.QueryRow(ctx, `SELECT CASE WHEN c.founder_key='founder-02' AND COALESCE(r.points,0)<20 THEN 2 ELSE 0 END FROM stations s LEFT JOIN companies c ON c.id=s.founder LEFT JOIN reputation r ON r.company_id=s.founder AND r.user_id=$1 WHERE s.star=$2 AND s.object_id=$3`, uid, star, id).Scan(&fee)
	return fee
}
func feeOffer(o Offer, percent int) Offer {
	if percent > 0 {
		o.Buy += (o.Buy*percent + 99) / 100
		o.Sell = max(0, o.Sell-(o.Sell*percent+99)/100)
	}
	return o
}

// Seed the immutable station catalogue once per loaded universe, not on every poll.
func (a *App) seedStations(ctx context.Context, g *Galaxy) error {
	a.stationMu.Lock()
	defer a.stationMu.Unlock()
	if a.stationCount == len(g.Stars) {
		return nil
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = ensureStations(ctx, tx, g); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	a.stationCount = len(g.Stars)
	return nil
}
