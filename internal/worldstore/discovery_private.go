package worldstore

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/universe"
)

func SecureDiscovery(ctx context.Context, tx pgx.Tx, g *universe.Galaxy) (bool, error) {
	if g.DiscoverySecret != "" {
		return false, nil
	}
	key := make([]byte, 32)
	if _, err := crand.Read(key); err != nil {
		return false, err
	}
	g.DiscoverySecret = hex.EncodeToString(key)
	// Existing observations and object-bound work must survive the upgrade.
	rows, err := tx.Query(ctx, `SELECT star FROM surveys UNION SELECT star FROM research_jobs UNION SELECT star FROM mining_jobs UNION SELECT star FROM resource_quotas UNION SELECT star FROM ships UNION SELECT source_star FROM automation_tasks WHERE status IN('queued','running','paused') UNION SELECT star FROM navigation_notes WHERE object_id<>'' UNION SELECT star FROM planet_surfaces`)
	if err != nil {
		return false, err
	}
	keep := map[int]bool{1: true, 5: true}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		keep[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for i := range g.Stars {
		universe.SealStar(&g.Stars[i], g.DiscoverySecret, keep[g.Stars[i].ID])
	}
	g.Revision++
	return true, nil
}
