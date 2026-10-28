package server

import (
	"encoding/json"
	"math/rand"
	"strconv"
)

type ResourceDetails struct {
	Code        string   `json:"code"`
	Rarity      string   `json:"rarity"`
	Description string   `json:"description"`
	Sources     string   `json:"sources"`
	Uses        []string `json:"uses"`
}

var resourceDetails = map[string]ResourceDetails{
	"orichalcum": {"Ti", "Редкий", "Титаносодержащее сырьё для лёгких и прочных сплавов.", "Редкие металлические жилы и астероиды.", []string{"Металлургия", "Строительство"}},
	"vajra":      {"W", "Редкий", "Тугоплавкий металл для промышленных резцов и нагруженных узлов.", "Редкие включения в каменистых мирах и астероидах.", []string{"Инструменты", "Металлургия"}},
	"azoth":      {"N", "Необычный", "Азотное сырьё для химического синтеза и питательных сред.", "Ледяные тела, газовые гиганты и химические станции.", []string{"Химия", "Агропроизводство"}},
	"agni":       {"H", "Обычный", "Стабилизированный водородный концентрат для корабельных топливных систем.", "Ледяные тела и атмосферы газовых гигантов.", []string{"Топливо", "Энергетика"}},
	"ambrosia":   {"BIO", "Редкий", "Редкая клеточная культура с ценными биополимерами для медицины и питания.", "Биологические месторождения садовых миров.", []string{"Питание", "Медицина"}},
	"manna":      {"P", "Обычный", "Фосфатное сырьё для удобрений, гидропоники и биопроизводства.", "Почвы садовых миров и агрокомплексы.", []string{"Агропроизводство", "Химия"}},
	"iron":       {"Fe", "Обычный", "Железосодержащее сырьё для конструкционных материалов.", "Каменистые планеты и металлические астероиды.", []string{"Металлургия", "Строительство"}},
	"carbon":     {"C", "Обычный", "Углеродное сырьё для химии, фильтров и композитов.", "Углеродистые астероиды и некоторые планеты.", []string{"Химия", "Композиты"}},
	"water":      {"H2O", "Обычный", "Водяной лёд для жизнеобеспечения и переработки.", "Ледяные поля, холодные и садовые миры.", []string{"Жизнеобеспечение", "Химия"}},
	"silicon":    {"Si", "Обычный", "Силикатное сырьё для стекла и электроники.", "Каменистые планеты и силикатные астероиды.", []string{"Электроника", "Стекло"}},
	"adamant":    {"KST", "Необычный", "Конструкционный сплав с углеродным армированием для тяжёлых корпусов.", "Промышленные и металлургические станции.", []string{"Строительство", "Корпуса"}},
	"electrum":   {"ELC", "Необычный", "Проводящий сплав золота и серебра для контактов и энергетических сборок.", "Промышленные станции и торговые узлы.", []string{"Электроника", "Энергетика"}},
	"nectar":     {"RAT", "Обычный", "Пищевой концентрат для длительных рейсов и станционных рационов.", "Продуктовые станции и торговые узлы.", []string{"Питание", "Жизнеобеспечение"}},
	"soma":       {"K", "Необычный", "Калийсодержащее сырьё для удобрений и биореакторных сред.", "Агрокомплексы и химические станции.", []string{"Агропроизводство", "Химия"}},
}

func resourceMeta(id string) ResourceDetails { return resourceDetails[id] }
func resourceCatalog() []map[string]any {
	out := []map[string]any{}
	for _, r := range resources {
		raw, _ := json.Marshal(r)
		var item map[string]any
		_ = json.Unmarshal(raw, &item)
		raw, _ = json.Marshal(resourceMeta(r.ID))
		var meta map[string]any
		_ = json.Unmarshal(raw, &meta)
		for k, v := range meta {
			item[k] = v
		}
		out = append(out, item)
	}
	return out
}
func variedResources(star, object int, kind string) []string {
	return variedStarResources(Star{ID: star}, object, kind)
}
func variedStarResources(s Star, object int, kind string) []string {
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
func upgradeGeology(g *Galaxy) {
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
			p.Resources = variedStarResources(g.Stars[i], p.ID, p.Kind)
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
