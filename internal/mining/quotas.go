package mining

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/gameplay"
	resourcecatalog "github.com/yomezza/cosmosimulator/internal/resources"
	"github.com/yomezza/cosmosimulator/internal/universe"
)

func quotaCapacity(g gameplay.Settings, star, object int, res string) int {
	if res == "*" {
		return g.Common
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%d:%d:%s", star, object, res)
	return g.RareMin + int(h.Sum32()%uint32(g.RareMax-g.RareMin+1))
}

func EnsureQuota(ctx context.Context, tx pgx.Tx, g gameplay.Settings, star, object int, res string, now time.Time, base *resourcecatalog.Quota) (resourcecatalog.Quota, error) {
	var q resourcecatalog.Quota
	start := g.Epoch.Add(time.Duration(math.Floor(now.Sub(g.Epoch).Seconds()/float64(g.ResetSeconds))*float64(g.ResetSeconds)) * time.Second)
	end := start.Add(time.Duration(g.ResetSeconds) * time.Second)
	if base != nil {
		start, end = base.Start, base.ResetAt
	}
	cap := quotaCapacity(g, star, object, res)
	_, err := tx.Exec(ctx, `INSERT INTO resource_quotas(star,object,resource,capacity,window_start,reset_at,generation) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, star, object, res, cap, start, end, g.Generation)
	if err != nil {
		return q, err
	}
	var used int
	var generation int64
	err = tx.QueryRow(ctx, `SELECT capacity,used,window_start,reset_at,generation FROM resource_quotas WHERE star=$1 AND object=$2 AND resource=$3 FOR UPDATE`, star, object, res).Scan(&q.Capacity, &used, &q.Start, &q.ResetAt, &generation)
	if err != nil {
		return q, err
	}
	if generation != g.Generation || !now.Before(q.ResetAt) || base != nil && !q.Start.Equal(base.Start) {
		if generation != g.Generation {
			q.Start = start
			q.ResetAt = end
		} else if base != nil {
			q.Start, q.ResetAt = base.Start, base.ResetAt
		} else {
			elapsed := now.Sub(q.ResetAt)
			q.Start = q.ResetAt.Add(time.Duration(math.Floor(elapsed.Seconds()/float64(g.ResetSeconds))*float64(g.ResetSeconds)) * time.Second)
			q.ResetAt = q.Start.Add(time.Duration(g.ResetSeconds) * time.Second)
		}
		q.Capacity = cap
		used = 0
		if _, err = tx.Exec(ctx, `UPDATE resource_quotas SET capacity=$4,used=0,window_start=$5,reset_at=$6,generation=$7 WHERE star=$1 AND object=$2 AND resource=$3`, star, object, res, q.Capacity, q.Start, q.ResetAt, g.Generation); err != nil {
			return q, err
		}
	}
	q.Remaining = q.Capacity - used
	return q, nil
}

func reserveExtraction(ctx context.Context, tx pgx.Tx, star, object int, res string, limit ...int) (int, time.Time, error) {
	g, err := gameplay.ReadGameplay(ctx, tx)
	if err != nil {
		return 0, time.Time{}, err
	}
	now := time.Now()
	common, err := EnsureQuota(ctx, tx, g, star, object, "*", now, nil)
	if err != nil {
		return 0, now, err
	}
	remaining := common.Remaining
	if resourcecatalog.Meta(res).Rarity == "Редкий" {
		rare, err := EnsureQuota(ctx, tx, g, star, object, res, now, &common)
		if err != nil {
			return 0, now, err
		}
		remaining = min(remaining, rare.Remaining)
	}
	batch := 3
	if len(limit) > 0 {
		batch = limit[0]
	}
	count := min(batch, remaining)
	if count <= 0 {
		return 0, common.Start, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE resource_quotas SET used=used+$4 WHERE star=$1 AND object=$2 AND resource IN ('*',$3)`, star, object, res, count); err != nil {
		return 0, common.Start, err
	}
	return count, common.Start, nil
}

func QuotaView(ctx context.Context, tx pgx.Tx, s universe.Star, sites []universe.Site) ([]universe.Site, error) {
	g, err := gameplay.ReadGameplay(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range sites {
		if len(sites[i].Resources) == 0 {
			continue
		}
		common, err := EnsureQuota(ctx, tx, g, s.ID, sites[i].Planet, "*", now, nil)
		if err != nil {
			return nil, err
		}
		sites[i].Quality = resourcecatalog.Quality(s.ID, sites[i].Planet)
		sites[i].Concentration = resourcecatalog.Concentration(s.ID, sites[i].Planet)
		sites[i].SharedQuota = &common
		sites[i].Quotas = map[string]resourcecatalog.Quota{}
		for _, res := range sites[i].Resources {
			q := common
			if resourcecatalog.Meta(res).Rarity == "Редкий" {
				q, err = EnsureQuota(ctx, tx, g, s.ID, sites[i].Planet, res, now, &common)
				if err != nil {
					return nil, err
				}
				q.Remaining = min(q.Remaining, common.Remaining)
			}
			sites[i].Quotas[res] = q
		}
	}
	return sites, nil
}
