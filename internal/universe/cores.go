package universe

import (
	"math"
	"sort"
)

func CoreSystems(g *Galaxy, galaxies ...string) []int {
	key := "edem"
	if len(galaxies) > 0 {
		key = galaxies[0]
	}
	list := append([]Star(nil), g.Stars...)
	sort.Slice(list, func(i, j int) bool { return math.Hypot(list[i].X, list[i].Y) < math.Hypot(list[j].X, list[j].Y) })
	out := []int{}
	for _, s := range list {
		if s.Deleted || GalaxyKey(s) != key || s.Region < 0 {
			continue
		}
		out = append(out, s.ID)
		if len(out) == 6 {
			break
		}
	}
	return out
}
