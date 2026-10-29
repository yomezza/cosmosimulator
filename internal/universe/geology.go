package universe

import (
	"math/rand"
	"strconv"
)

func VariedResources(star, object int, kind string) []string {
	return VariedStarResources(Star{ID: star}, object, kind)
}

func VariedStarResources(s Star, object int, kind string) []string {
	star := s.ID
	rng := rand.New(rand.NewSource(int64(star)*104729 + int64(object)*7919))
	if s.DiscoverySeed != "" {
		rng = discoveryRNG(s.DiscoverySeed, "resources/"+strconv.Itoa(object))
	}
	out := []string{}
	add := func(id string, chance float64) {
		if s.ResourceWeights != nil {
			chance *= float64(s.ResourceWeights[id]) / 100
		}
		chance = max(0, min(1, chance))
		roll := rng.Float64()
		if saved, ok := s.ResourceRolls[strconv.Itoa(object)][id]; ok {
			roll = saved
		}
		if roll < chance {
			out = append(out, id)
		}
	}
	for _, item := range resourceChances(kind) {
		add(item.id, item.chance)
	}
	if len(out) == 0 && s.ResourceWeights == nil {
		if kind == "Ледяная" || kind == "Ледяное поле" {
			out = append(out, "water")
		} else if kind == "Газовый гигант" {
			out = append(out, "agni")
		} else {
			out = append(out, "silicon")
		}
	}
	if star == 1 && object == 101 {
		out = []string{}
		for _, id := range []string{"iron", "carbon", "orichalcum"} {
			if s.ResourceWeights == nil || s.ResourceWeights[id] > 0 {
				out = append(out, id)
			}
		}
	}
	return out
}

func UpgradeGeology(g *Galaxy) {
	if g.Version >= 3 {
		return
	}
	for i := range g.Stars {
		for j := range g.Stars[i].Planets {
			p := &g.Stars[i].Planets[j]
			if p.Kind == "Каменистая" && p.Temperature >= 230 && p.Temperature <= 320 && p.Atmosphere == "Плотная" {
				p.Kind = "Садовый мир"
			}
			if p.Kind == "Каменистая" && p.Temperature > 700 {
				p.Kind = "Вулканическая"
			}
			p.Resources = VariedStarResources(g.Stars[i], p.ID, p.Kind)
		}
	}
	g.Version = 3
}

type resourceChance struct {
	id     string
	chance float64
}

func resourceChances(kind string) []resourceChance {
	switch kind {
	case "Ледяная", "Ледяное поле":
		return []resourceChance{{"water", .85}, {"carbon", .3}, {"agni", .65}, {"azoth", .18}}
	case "Газовый гигант":
		return []resourceChance{{"agni", .85}, {"carbon", .6}, {"azoth", .25}}
	case "Садовый мир":
		return []resourceChance{{"water", .8}, {"manna", .85}, {"carbon", .5}, {"ambrosia", .12}}
	default:
		return []resourceChance{{"iron", .7}, {"silicon", .55}, {"carbon", .4}, {"orichalcum", .12}, {"vajra", .06}}
	}
}
