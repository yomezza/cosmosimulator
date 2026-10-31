package scenestore

import (
	"context"
	"math"
	"sort"

	"github.com/yomezza/cosmosimulator/internal/planet/scene"
)

// MaterialPoint contains only public nearby biome/UV data, never claims, seed,
// resource estimates or private geology. Geometry remains the saved scene v1.
type MaterialPoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Biome string  `json:"biome"`
}

func (r *Store) Materials(ctx context.Context, s *scene.Scene) ([]MaterialPoint, error) {
	src, err := readSource(ctx, r.db, s.Key())
	if err != nil {
		return nil, err
	}
	if src.checksum != s.SourceChecksum() {
		return nil, scene.ErrSource
	}
	center := s.Metadata().Center
	out := make([]MaterialPoint, 0, 19)
	for _, sector := range src.g.Sectors {
		if math.Hypot(sector.Center.X-center.X, sector.Center.Y-center.Y) <= 3.6 {
			out = append(out, MaterialPoint{sector.Center.X, sector.Center.Y, sector.Biome})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		da := math.Hypot(a.X-center.X, a.Y-center.Y)
		db := math.Hypot(b.X-center.X, b.Y-center.Y)
		if da == db {
			if a.Y == b.Y {
				return a.X < b.X
			}
			return a.Y < b.Y
		}
		return da < db
	})
	if len(out) > 19 {
		out = out[:19]
	}
	return out, nil
}
