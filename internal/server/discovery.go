package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// The scanner reveals objects in the same stable order used by startResearch.
// Hiding is enforced by the API, not by drawing invisible objects in the browser.
func revealedSites(ctx context.Context, q querier, uid int64, star int, sites []Site, known bool) ([]Site, error) {
	if known {
		return sites, nil
	}
	var n int
	err := q.QueryRow(ctx, `SELECT scanned FROM research_jobs WHERE user_id=$1 AND star=$2`, uid, star).Scan(&n)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	out := append([]Site{}, sites[:max(0, min(n, len(sites)))]...)
	// A corporation knows the ports it has built even before surveying planets.
	var owned []string
	if err = q.QueryRow(ctx, `SELECT ARRAY(SELECT s.object_id FROM stations s JOIN users u ON u.company_id=s.company_id WHERE u.id=$1 AND s.star=$2 AND s.active)`, uid, star).Scan(&owned); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range out {
		seen[s.ID] = true
	}
	for _, s := range sites {
		for _, id := range owned {
			if s.ID == id && !seen[id] {
				out = append(out, s)
				seen[id] = true
			}
		}
	}
	return out, nil
}
