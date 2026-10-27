package server

import (
	"encoding/json"
	"math/rand"
)

type ResourceDetails struct {
	Code        string   `json:"code"`
	Rarity      string   `json:"rarity"`
	Description string   `json:"description"`
	Sources     string   `json:"sources"`
	Uses        []string `json:"uses"`
}

var resourceDetails = map[string]ResourceDetails{
	"adamant":    {"ADM", "Необычный", "Жаропрочный конструкционный сплав для несущих узлов и тяжёлых обшивок.", "Готовые партии на промышленных и металлургических станциях.", []string{"Строительство", "Корпуса"}},
	"electrum":   {"ELC", "Необычный", "Проводящие сборки для энергетических шин и бортовой электроники.", "Промышленные станции и торговые узлы.", []string{"Электроника", "Энергетика"}},
	"nectar":     {"NCT", "Обычный", "Пищевой концентрат для длительных рейсов и станционных рационов.", "Продуктовые станции и торговые узлы.", []string{"Питание", "Жизнеобеспечение"}},
	"soma":       {"SOM", "Необычный", "Устойчивая биокультура для медицинских субстратов и биореакторов.", "Агрокомплексы и химические станции.", []string{"Медицина", "Агропроизводство"}},
	"orichalcum": {"ORI", "Редкий", "Прочный проводящий сплав рудного происхождения.", "Редкие металлические жилы и астероиды.", []string{"Металлургия", "Строительство"}},
	"vajra":      {"VJR", "Редкий", "Твёрдые кристаллы для точных режущих инструментов.", "Редкие кристаллические включения каменистых миров.", []string{"Инструменты", "Электроника"}},
	"azoth":      {"AZT", "Необычный", "Стабилизированный катализатор для переработки сложного сырья.", "Ледяные тела и атмосферы гигантов.", []string{"Химия", "Переработка"}},
	"agni":       {"AGN", "Обычный", "Стандартизированный топливный концентрат для двигателей.", "Летучие вещества ледяных тел и газовых гигантов.", []string{"Топливо", "Энергетика"}},
	"ambrosia":   {"AMB", "Редкий", "Ценная питательная биокультура.", "Редкие месторождения биосырья садовых миров.", []string{"Питание", "Медицина"}},
	"manna":      {"MNA", "Обычный", "Базовое пищевое биосырьё колоний.", "Садовые миры с подходящим температурным режимом.", []string{"Питание", "Агропроизводство"}},
	"iron":       {"FE", "Обычный", "Железосодержащее сырьё для конструкционных материалов.", "Часть каменистых планет и металлических астероидов.", []string{"Металлургия", "Строительство"}},
	"carbon":     {"C", "Обычный", "Углеродное сырьё для химии и композитов.", "Углеродистые астероиды и некоторые планеты.", []string{"Химия", "Композиты"}},
	"water":      {"H2O", "Обычный", "Водяной лёд для жизнеобеспечения и переработки.", "Ледяные поля, холодные и садовые миры.", []string{"Жизнеобеспечение", "Химия"}},
	"silicon":    {"SI", "Обычный", "Силикатное сырьё для стекла и электроники.", "Часть каменистых планет и астероидов.", []string{"Электроника", "Стекло"}},
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
	rng := rand.New(rand.NewSource(int64(star)*104729 + int64(object)*7919))
	out := []string{}
	add := func(id string, chance float64) {
		if rng.Float64() < chance {
			out = append(out, id)
		}
	}
	switch kind {
	case "Ледяная", "Ледяное поле":
		add("water", .85)
		add("carbon", .3)
		add("agni", .65)
		add("azoth", .18)
	case "Газовый гигант":
		add("agni", .85)
		add("carbon", .6)
		add("azoth", .25)
	case "Садовый мир":
		add("water", .8)
		add("manna", .85)
		add("carbon", .5)
		add("ambrosia", .12)
	default:
		add("iron", .7)
		add("silicon", .55)
		add("carbon", .4)
		add("orichalcum", .12)
		add("vajra", .06)
	}
	if len(out) == 0 {
		if kind == "Ледяная" || kind == "Ледяное поле" {
			out = append(out, "water")
		} else if kind == "Газовый гигант" {
			out = append(out, "agni")
		} else {
			out = append(out, "silicon")
		}
	}
	if star == 1 && object == 101 {
		return []string{"iron", "carbon", "orichalcum"}
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
			p.Resources = variedResources(g.Stars[i].ID, p.ID, p.Kind)
		}
	}
	g.Version = 3
}
