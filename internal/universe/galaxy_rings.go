package universe

import (
	"math"
)

// Reproject Edem-II only. IDs, regions, objects and hyperlanes stay unchanged,
// including routes already being flown when an existing universe is upgraded.
func ArrangeFrontierRings(g *Galaxy) bool {
	if len(g.Catalog) > 0 || g.RingLayoutVersion >= 1 || len(g.Stars) <= 1000 {
		return false
	}
	for i := range g.Stars {
		s := &g.Stars[i]
		if s.ID <= 1000 || s.ID > 2000 {
			continue
		}
		radius := math.Hypot(s.X, s.Y)
		angle := math.Atan2(s.Y, s.X)
		if s.Region >= 4 && s.Region <= 7 {
			// Unwrap each old spiral arm into three annular sectors. Nearby
			// systems retain their order within a belt, keeping corridors legible.
			position := max(0, min(2.999999, (radius-100)/(850.0/3)))
			belt := math.Floor(position)
			fraction := position - belt
			sector := float64(s.Region-4) * math.Pi / 2
			jitter := math.Remainder(angle-sector-radius/370, 2*math.Pi)
			angle = sector + fraction*math.Pi/2 + jitter*.3
			radius = 390 + belt*290 + jitter*220
		} else {
			// Isolated outer systems remain isolated outside the rings.
			radius = 1120 + max(0, min(150, radius-1050))*.5
		}
		s.X = math.Round(math.Cos(angle)*radius*100) / 100
		s.Y = math.Round(math.Sin(angle)*radius*100) / 100
	}
	g.RingLayoutVersion = 1
	return true
}
