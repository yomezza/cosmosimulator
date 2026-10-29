package worldstore

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
	"github.com/yomezza/cosmosimulator/internal/universe"
)

func AddCustomSites(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, s universe.Star, sites []universe.Site) ([]universe.Site, error) {
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
			sites = append(sites, universe.Site{ID: id, Name: name, Kind: "Орбитальная станция", Orbit: 2.1 + float64(len(sites))*.15})
		}
	}
	return sites, rows.Err()
}

func StationAllows(ctx context.Context, q postgres.Querier, star int, id, res string) bool {
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

// Port fees are included in the quote. Market wallets receive the same amount
// the player pays; no currency is created by the reputation discount.
func PortFee(ctx context.Context, q postgres.Querier, uid int64, star int, id string) int {
	var fee int
	_ = q.QueryRow(ctx, `SELECT CASE WHEN c.founder_key='founder-02' AND COALESCE(r.points,0)<20 THEN 2 ELSE 0 END FROM stations s LEFT JOIN companies c ON c.id=s.founder LEFT JOIN reputation r ON r.company_id=s.founder AND r.user_id=$1 WHERE s.star=$2 AND s.object_id=$3`, uid, star, id).Scan(&fee)
	return fee
}

// Seed the immutable station catalogue once per loaded universe, not on every poll.
func (a *Service) SeedStations(ctx context.Context, g *universe.Galaxy) error {
	a.stationMu.Lock()
	defer a.stationMu.Unlock()
	if a.stationCount == len(g.Stars) {
		return nil
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = EnsureStations(ctx, tx, g); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	a.stationCount = len(g.Stars)
	return nil
}
