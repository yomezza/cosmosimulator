package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type GalaxySettings struct {
	ExtraLanes      int            `json:"extra_lanes"`
	CrossLinks      int            `json:"cross_links"`
	MaxLaneLength   int            `json:"max_lane_length"`
	StationPercent  int            `json:"station_percent"`
	MaxPorts        int            `json:"max_ports"`
	ResourceWeights map[string]int `json:"resource_weights"`
}
type GalaxyDefinition struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Shape       string             `json:"shape"`
	Seed        int64              `json:"seed"`
	Opened      bool               `json:"opened"`
	Radius      float64            `json:"radius"`
	Settings    GalaxySettings     `json:"settings"`
	Territories []FounderTerritory `json:"territories,omitempty"`
}

func defaultGalaxySettings() GalaxySettings {
	weights := map[string]int{}
	for _, r := range resources {
		weights[r.ID] = 100
	}
	return GalaxySettings{15, 8, 1400, 18, 3, weights}
}
func galaxyKey(s Star) string {
	if s.Galaxy == "asterion" || s.Galaxy == "Edem-II" || s.Galaxy == "Астерион" {
		return "asterion"
	}
	return "edem"
}
func (g *Galaxy) definition(id string) *GalaxyDefinition {
	for i := range g.Catalog {
		if g.Catalog[i].ID == id {
			return &g.Catalog[i]
		}
	}
	return nil
}
func galaxyStars(g *Galaxy, id string) []Star {
	list := []Star{}
	for _, s := range g.Stars {
		if galaxyKey(s) == id {
			list = append(list, s)
		}
	}
	return list
}

// Establish explicit membership before adding any systems. Existing global IDs
// remain indexes into Stars, even when galaxies have interleaved ID ranges.
func initializeGalaxyCatalog(g *Galaxy) {
	if len(g.Catalog) > 0 {
		return
	}
	arrangeFrontierRings(g)
	g.Catalog = []GalaxyDefinition{{ID: "edem", Name: "Edem", Shape: "spiral", Seed: g.Seed, Opened: true, Radius: 3600, Settings: defaultGalaxySettings()}, {ID: "asterion", Name: "Астерион", Shape: "rings", Seed: 20261027, Radius: 1250, Settings: defaultGalaxySettings()}}
	serials := map[string]int{}
	for i := range g.Stars {
		s := &g.Stars[i]
		id := galaxyKey(*s)
		serials[id]++
		s.Galaxy = id
		s.Number = serials[id]
		if id == "asterion" {
			g.Catalog[1].Opened = true
		}
		if id == "edem" {
			s.X *= 3
			s.Y *= 3
		}
	}
}

func appendGalaxySystems(g *Galaxy, id string, target int) error {
	d := g.definition(id)
	if d == nil {
		return errors.New("Галактика не найдена")
	}
	current := len(galaxyStars(g, id))
	if target < current || target > 10000 {
		return errors.New("Количество систем можно только увеличивать, максимум 10000")
	}
	if current == target {
		return nil
	}
	// New objects get globally unique IDs; generation of existing objects is never repeated.
	generated := generateGalaxy(target-current, d.Seed+int64(current)*131)
	rng := rand.New(rand.NewSource(d.Seed + int64(current)*104729))
	used := map[string]bool{}
	for _, s := range g.Stars {
		used[s.Name] = true
	}
	for i, s := range generated.Stars {
		s.ID = len(g.Stars) + 1
		s.Number = current + i + 1
		s.Galaxy = id
		region := (s.Number - 1) % 4
		if id == "asterion" {
			region += 4
		}
		s.Region = region
		if s.Number%50 == 0 {
			s.Region = -1
			if id == "asterion" {
				s.Region = -2
			}
		}
		radius := 300 + math.Sqrt(rng.Float64())*2400
		angle := float64(region%4)*math.Pi/2 + radius/1110 + (rng.Float64()-.5)*.5
		if id == "asterion" {
			radius = 390 + float64(s.Number%3)*290 + (rng.Float64()-.5)*70
			angle = rng.Float64() * 2 * math.Pi
		}
		if s.Region < 0 {
			radius = d.Radius * (.9 + rng.Float64()*.08)
			angle = rng.Float64() * 2 * math.Pi
		}
		// Best-of-candidates spacing avoids adding stars directly on existing dots.
		bestX, bestY, bestDistance := 0.0, 0.0, -1.0
		for trial := 0; trial < 8; trial++ {
			x, y := math.Cos(angle)*radius, math.Sin(angle)*radius
			nearest := math.Inf(1)
			for _, p := range g.Stars {
				if galaxyKey(p) == id {
					nearest = math.Min(nearest, math.Hypot(x-p.X, y-p.Y))
				}
			}
			if nearest > bestDistance {
				bestX, bestY, bestDistance = x, y, nearest
			}
			if nearest >= 32 {
				break
			}
			angle += (rng.Float64() - .5) * .08
			radius += (rng.Float64() - .5) * 30
		}
		s.X = math.Round(bestX*100) / 100
		s.Y = math.Round(bestY*100) / 100
		prefix := "ED"
		if id == "asterion" {
			prefix = "AS"
		}
		s.Name = fmt.Sprintf("%s-%04d", prefix, s.Number)
		if s.Number%5 == 1 {
			s.Name = fmt.Sprintf("KE-%04d", s.ID)
		}
		if s.Number%5 == 2 {
			s.Name = fmt.Sprintf("AX-%04d", s.ID)
		}
		if s.Number%5 >= 3 {
			s.Name = fmt.Sprintf("%s %d", []string{"Дальняя Заря", "Янтарный Предел", "Серебряная Река", "Тихая Лагуна", "Лазурная Искра", "Звёздная Колыбель", "Скрытая Обитель", "Вечерняя Гавань"}[s.Number%8], 1+s.Number/40)
		}
		if used[s.Name] {
			s.Name += fmt.Sprintf(" · %d", s.ID)
		}
		used[s.Name] = true
		for j := range s.Planets {
			s.Planets[j].Name = fmt.Sprintf("%s-%c", s.Name, 'b'+j)
		}
		g.Stars = append(g.Stars, s)
	}
	d.Opened = true
	return nil
}

func assignGalaxyContent(g *Galaxy, id string) {
	d := g.definition(id)
	core := map[int]bool{}
	for _, v := range coreSystems(g) {
		core[v] = true
	}
	for i := range g.Stars {
		s := &g.Stars[i]
		if galaxyKey(*s) != id {
			continue
		}
		n := 0
		if stablePercent(d.Seed, s.ID, 1) < founderStationChance(*d, *s) {
			n = 1 + stablePercent(d.Seed, s.ID, 2)%d.Settings.MaxPorts
		}
		if core[s.ID] {
			n = max(1, n)
		}
		if s.ID == 1 || s.ID == 5 {
			n = max(3, n)
		}
		s.Ports = &n
		s.ResourceWeights = d.Settings.ResourceWeights
		for j := range s.Planets {
			s.Planets[j].Resources = variedStarResources(*s, s.Planets[j].ID, s.Planets[j].Kind)
		}
	}
	assignFounderTerritories(g, id)
}
func stablePercent(seed int64, id, salt int) int {
	return int(uint64(seed+int64(id)*104729+int64(salt)*7919)*6364136223846793005>>32) % 100
}

func rebuildGalaxyLanes(g *Galaxy, id string, protected []Lane) {
	d := g.definition(id)
	stars := galaxyStars(g, id)
	lanes := []Lane{}
	seen := map[[2]int]bool{}
	add := func(a, b int) {
		if a > b {
			a, b = b, a
		}
		key := [2]int{a, b}
		if a != b && !seen[key] {
			seen[key] = true
			lanes = append(lanes, Lane{a, b})
		}
	}
	for _, e := range g.Lanes {
		if galaxyKey(g.Stars[e.From-1]) != id {
			add(e.From, e.To)
		}
	}
	// A minimum spanning forest favors short corridors; isolated systems stay out.
	for region := 0; region < 8; region++ {
		members := []Star{}
		for _, s := range stars {
			if s.Region == region {
				members = append(members, s)
			}
		}
		if len(members) < 2 {
			continue
		}
		inTree := make([]bool, len(members))
		distance := make([]float64, len(members))
		parent := make([]int, len(members))
		for i := range distance {
			distance[i] = math.Inf(1)
			parent[i] = -1
		}
		distance[0] = 0
		for range members {
			u := -1
			for j := range members {
				if !inTree[j] && (u < 0 || distance[j] < distance[u]) {
					u = j
				}
			}
			inTree[u] = true
			if parent[u] >= 0 && distance[u] <= float64(d.Settings.MaxLaneLength) {
				add(members[u].ID, members[parent[u]].ID)
			}
			for j := range members {
				if !inTree[j] {
					v := math.Hypot(members[u].X-members[j].X, members[u].Y-members[j].Y)
					if v < distance[j] {
						distance[j] = v
						parent[j] = u
					}
				}
			}
		}
		for _, s := range members {
			if stablePercent(d.Seed, s.ID, 3) >= d.Settings.ExtraLanes {
				continue
			}
			best := 0
			dist := float64(d.Settings.MaxLaneLength)
			for _, p := range members {
				a, b := s.ID, p.ID
				if a > b {
					a, b = b, a
				}
				v := math.Hypot(s.X-p.X, s.Y-p.Y)
				if p.ID != s.ID && !seen[[2]int{a, b}] && v < dist {
					best = p.ID
					dist = v
				}
			}
			if best > 0 {
				add(s.ID, best)
			}
		}
	}
	// Keep only the nearest candidates per adjacent arm pair in one pass.
	// Increasing the requested bridge count does not repeat the all-pairs search.
	type bridgeCandidate struct {
		a, b     int
		distance float64
	}
	candidates := [4][]bridgeCandidate{}
	if d.Settings.CrossLinks > 0 {
		limit := (d.Settings.CrossLinks + 3) / 4
		for arm := 0; arm < 4; arm++ {
			for _, a := range stars {
				if a.Region < 0 || a.Region%4 != arm {
					continue
				}
				for _, b := range stars {
					if b.Region < 0 || b.Region%4 != (arm+1)%4 {
						continue
					}
					x, y := a.ID, b.ID
					if x > y {
						x, y = y, x
					}
					if seen[[2]int{x, y}] {
						continue
					}
					distance := math.Hypot(a.X-b.X, a.Y-b.Y)
					if distance > float64(d.Settings.MaxLaneLength) {
						continue
					}
					list := candidates[arm]
					if len(list) == limit && distance >= list[len(list)-1].distance {
						continue
					}
					list = append(list, bridgeCandidate{a.ID, b.ID, distance})
					sort.Slice(list, func(i, j int) bool { return list[i].distance < list[j].distance })
					if len(list) > limit {
						list = list[:limit]
					}
					candidates[arm] = list
				}
			}
		}
		for i := 0; i < d.Settings.CrossLinks; i++ {
			list := candidates[i%4]
			index := i / 4
			if index < len(list) {
				add(list[index].a, list[index].b)
			}
		}
	}

	for _, e := range protected {
		add(e.From, e.To)
	}
	// Keep the two original training links usable by starter hulls. They are
	// explicit exceptions to the generated length limit, like committed flights.
	if id == "edem" && len(g.Stars) >= 9 {
		add(1, 5)
		add(5, 9)
	}
	sort.Slice(lanes, func(i, j int) bool {
		if lanes[i].From == lanes[j].From {
			return lanes[i].To < lanes[j].To
		}
		return lanes[i].From < lanes[j].From
	})
	g.Lanes = lanes
}

func protectedTravelLanes(ctx context.Context, tx pgx.Tx, g *Galaxy) ([]Lane, error) {
	rows, err := tx.Query(ctx, `SELECT star,COALESCE(destination,star),travel_path,waypoints FROM ships ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lane{}
	for rows.Next() {
		var star, to int
		var raw, queued []byte
		var path, waypoints []int
		if err = rows.Scan(&star, &to, &raw, &queued); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &path); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(queued, &waypoints); err != nil {
			return nil, err
		}
		for i := 1; i < len(path); i++ {
			out = append(out, Lane{path[i-1], path[i]})
		}
		from := to
		for _, next := range waypoints {
			p := g.path(from, next)
			for i := 1; i < len(p); i++ {
				out = append(out, Lane{p[i-1], p[i]})
			}
			from = next
		}
	}
	return out, rows.Err()
}

// New shortcuts may reduce the fuel needed by queued legs. Release the surplus
// reservation now so the ship does not finish its route with fuel locked forever.
func repriceQueuedRoutes(ctx context.Context, tx pgx.Tx, g *Galaxy) error {
	rows, err := tx.Query(ctx, `SELECT id,COALESCE(destination,star),waypoints,class,fuel FROM ships WHERE reserved_fuel<>0 OR jsonb_array_length(waypoints)>0 ORDER BY id`)
	if err != nil {
		return err
	}
	type reservation struct {
		id   int64
		cost int
	}
	list := []reservation{}
	for rows.Next() {
		var sid int64
		var from, fuel int
		var raw []byte
		var class string
		if err = rows.Scan(&sid, &from, &raw, &class, &fuel); err != nil {
			rows.Close()
			return err
		}
		var points []int
		if err = json.Unmarshal(raw, &points); err != nil {
			rows.Close()
			return err
		}
		cost := 0
		for _, to := range points {
			path := g.path(from, to)
			if len(path) == 0 {
				rows.Close()
				return errors.New("Сохранённый маршрут недоступен")
			}
			cost += (len(path) - 1) * (Ship{Class: class}).hull().JumpFuel
			from = to
		}
		if cost > fuel {
			rows.Close()
			return errors.New("Недостаточно топлива для сохранённого маршрута")
		}
		list = append(list, reservation{sid, cost})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range list {
		if _, err = tx.Exec(ctx, `UPDATE ships SET reserved_fuel=$2 WHERE id=$1`, r.id, r.cost); err != nil {
			return err
		}
	}
	return nil
}

// Ports with player property or live obligations are exempt from redistribution.
func protectedPorts(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `SELECT star,object_id FROM stations s WHERE company_id IS NOT NULL
 OR EXISTS(SELECT 1 FROM ships p WHERE p.star=s.star AND (p.object_id=s.object_id OR p.local_target=s.object_id))
 OR EXISTS(SELECT 1 FROM warehouses w WHERE w.star=s.star AND w.station=s.object_id AND w.cargo<>'{}')
 OR EXISTS(SELECT 1 FROM company_storage w WHERE w.star=s.star AND w.station=s.object_id AND w.cargo<>'{}')
 OR EXISTS(SELECT 1 FROM reservations r WHERE r.star=s.star AND r.station=s.object_id AND r.status='reserved')
 OR EXISTS(SELECT 1 FROM sell_orders o WHERE o.star=s.star AND o.station=s.object_id AND o.quantity>0)
 OR EXISTS(SELECT 1 FROM survey_offers o WHERE o.star=s.star AND o.station=s.object_id)
 OR EXISTS(SELECT 1 FROM deliveries d WHERE d.status IN('open','carrying','bot') AND ((d.from_star=s.star AND d.from_station=s.object_id) OR (d.to_star=s.star AND d.to_station=s.object_id)))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var star int
		var object string
		if err = rows.Scan(&star, &object); err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%d/%s", star, object)] = true
	}
	return out, rows.Err()
}
func plannedPorts(g *Galaxy, protected map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key := range protected {
		out[key] = true
	}
	for _, s := range g.Stars {
		for _, site := range systemSites(s) {
			if site.Kind == "Орбитальная станция" {
				out[fmt.Sprintf("%d/%s", s.ID, site.ID)] = true
			}
		}
	}
	return out
}
func reconcileStations(ctx context.Context, tx pgx.Tx, g *Galaxy) error {
	protected, err := protectedPorts(ctx, tx)
	if err != nil {
		return err
	}
	if err = ensureStations(ctx, tx, g); err != nil {
		return err
	}
	// Protected old ports keep their inventories and obligations. A public port
	// outside today's colonies becomes neutral rather than claiming the frontier.
	owners := make([]map[string]any, 0, len(g.Stars))
	for _, s := range g.Stars {
		owners = append(owners, map[string]any{"star": s.ID, "founder_key": s.Founder})
	}
	raw, err := json.Marshal(owners)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE stations s SET founder=c.id FROM jsonb_to_recordset($1) AS x(star int,founder_key text) LEFT JOIN companies c ON c.founder_key=x.founder_key WHERE s.star=x.star AND s.company_id IS NULL`, raw); err != nil {
		return err
	}
	keys := []string{}
	for key := range plannedPorts(g, protected) {
		keys = append(keys, key)
	}
	_, err = tx.Exec(ctx, `UPDATE stations SET active=(star::text||'/'||object_id)=ANY($1::text[])`, keys)
	return err
}

func upgradeManagedGalaxies(ctx context.Context, tx pgx.Tx, g *Galaxy) (bool, error) {
	if len(g.Catalog) > 0 {
		return false, nil
	}
	initializeGalaxyCatalog(g)
	protected, err := protectedTravelLanes(ctx, tx, g)
	if err != nil {
		return false, err
	}
	if err = appendGalaxySystems(g, "edem", 3000); err != nil {
		return false, err
	}
	for _, d := range g.Catalog {
		if d.Opened {
			assignGalaxyContent(g, d.ID)
			rebuildGalaxyLanes(g, d.ID, protected)
		}
	}
	g.Revision = 1
	g.SettlementVersion = 1
	if err = repriceQueuedRoutes(ctx, tx, g); err != nil {
		return false, err
	}
	return true, reconcileStations(ctx, tx, g)
}

type GalaxyEdit struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Systems  int            `json:"systems"`
	Settings GalaxySettings `json:"settings"`
	Preview  string         `json:"preview"`
}

func validateGalaxyEdit(in GalaxyEdit, g *Galaxy) error {
	d := g.definition(in.ID)
	if d == nil || !d.Opened {
		return errors.New("Галактика не открыта")
	}
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > 48 {
		return errors.New("Название должно содержать от 1 до 48 символов")
	}
	for _, other := range g.Catalog {
		if other.ID != in.ID && strings.EqualFold(other.Name, strings.TrimSpace(in.Name)) {
			return errors.New("Название галактики уже используется")
		}
	}
	s := in.Settings
	if s.ExtraLanes < 0 || s.ExtraLanes > 100 || s.CrossLinks < 0 || s.CrossLinks > 40 || s.MaxLaneLength < 50 || s.MaxLaneLength > 10000 || s.StationPercent < 0 || s.StationPercent > 100 || s.MaxPorts < 1 || s.MaxPorts > 5 {
		return errors.New("Некорректные параметры галактики")
	}
	if len(s.ResourceWeights) != len(resources) {
		return errors.New("Укажите распространённость каждого ресурса")
	}
	for _, r := range resources {
		v, ok := s.ResourceWeights[r.ID]
		if !ok || v < 0 || v > 300 {
			return errors.New("Распространённость ресурса должна быть от 0 до 300%")
		}
	}
	if in.Systems < len(galaxyStars(g, in.ID)) || in.Systems > 10000 {
		return errors.New("Количество систем можно только увеличивать, максимум 10000")
	}
	return nil
}
func buildGalaxyEdit(ctx context.Context, tx pgx.Tx, g *Galaxy, in GalaxyEdit) (*Galaxy, map[string]bool, error) {
	if err := validateGalaxyEdit(in, g); err != nil {
		return nil, nil, err
	}
	raw, _ := json.Marshal(g)
	var next Galaxy
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil, nil, err
	}
	d := next.definition(in.ID)
	d.Name = strings.TrimSpace(in.Name)
	d.Settings = in.Settings
	protected, err := protectedTravelLanes(ctx, tx, g)
	if err != nil {
		return nil, nil, err
	}
	if err = appendGalaxySystems(&next, in.ID, in.Systems); err != nil {
		return nil, nil, err
	}
	assignGalaxyContent(&next, in.ID)
	rebuildGalaxyLanes(&next, in.ID, protected)
	next.Revision = g.Revision + 1
	ports, err := protectedPorts(ctx, tx)
	return &next, ports, err
}
func previewDigest(g *Galaxy, ports map[string]bool) string {
	raw, _ := json.Marshal([]any{g, ports})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func galaxyStatistics(g *Galaxy, id string, ports map[string]bool) map[string]any {
	stars := galaxyStars(g, id)
	claimed := 0
	for _, s := range stars {
		if s.Founder != "" {
			claimed++
		}
	}
	lanes, cross, stationSystems, stationCount := 0, 0, map[int]bool{}, 0
	for _, e := range g.Lanes {
		if galaxyKey(g.Stars[e.From-1]) == id {
			lanes++
			if g.Stars[e.From-1].Region != g.Stars[e.To-1].Region {
				cross++
			}
		}
	}
	for key := range plannedPorts(g, ports) {
		var star int
		fmt.Sscanf(key, "%d/", &star)
		if star > 0 && galaxyKey(g.Stars[star-1]) == id {
			stationCount++
			stationSystems[star] = true
		}
	}
	resourceSystems := map[string]int{}
	for _, s := range stars {
		seen := map[string]bool{}
		for _, site := range systemSites(s) {
			for _, r := range site.Resources {
				seen[r] = true
			}
		}
		for r := range seen {
			resourceSystems[r]++
		}
	}
	return map[string]any{"systems": len(stars), "lanes": lanes, "cross_links": cross, "station_systems": len(stationSystems), "stations": stationCount, "resources": resourceSystems, "founder_systems": claimed, "neutral_systems": len(stars) - claimed}
}
func (a *App) adminGalaxies(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	apply := r.URL.Path == "/api/admin/galaxies/apply"
	if apply {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
			a.internal(w, err)
			return
		}
	}
	var raw []byte
	sql := `SELECT data FROM galaxies WHERE name='Edem'`
	if apply {
		sql += " FOR UPDATE"
	}
	if err = tx.QueryRow(ctx, sql).Scan(&raw); err != nil {
		a.internal(w, err)
		return
	}
	var g Galaxy
	if err = json.Unmarshal(raw, &g); err != nil {
		a.internal(w, err)
		return
	}
	if r.Method == "GET" {
		ports, e := protectedPorts(ctx, tx)
		if e != nil {
			a.internal(w, e)
			return
		}
		list := []map[string]any{}
		for _, d := range g.Catalog {
			list = append(list, map[string]any{"definition": d, "stats": galaxyStatistics(&g, d.ID, ports)})
		}
		reply(w, 200, map[string]any{"galaxies": list, "revision": g.Revision})
		return
	}
	var in GalaxyEdit
	if !decode(w, r, &in) {
		return
	}
	next, ports, err := buildGalaxyEdit(ctx, tx, &g, in)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	digest := previewDigest(next, ports)
	if !apply {
		points := galaxyStars(next, in.ID)
		for i := range points {
			points[i].Planets = nil
			points[i].ResourceWeights = nil
		}
		lanes := []Lane{}
		for _, e := range next.Lanes {
			if galaxyKey(next.Stars[e.From-1]) == in.ID {
				lanes = append(lanes, e)
			}
		}
		reply(w, 200, map[string]any{"preview": digest, "before": galaxyStatistics(&g, in.ID, ports), "after": galaxyStatistics(next, in.ID, ports), "stars": points, "lanes": lanes, "protected_ports": len(ports), "radius": next.definition(in.ID).Radius, "territories": next.definition(in.ID).Territories})
		return
	}
	if in.Preview == "" || in.Preview != digest {
		fail(w, 409, "Карта или задания изменились. Сформируйте предпросмотр заново")
		return
	}
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&role); err != nil {
		a.internal(w, err)
		return
	}
	if role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	if err = reconcileStations(ctx, tx, next); err != nil {
		a.internal(w, err)
		return
	}
	if err = repriceQueuedRoutes(ctx, tx, next); err != nil {
		a.internal(w, err)
		return
	}
	raw, _ = json.Marshal(next)
	if _, err = tx.Exec(ctx, `UPDATE galaxies SET data=$1 WHERE name='Edem'`, raw); err != nil {
		a.internal(w, err)
		return
	}
	audit, _ := json.Marshal(map[string]any{"galaxy": in.ID, "name": in.Name, "before": galaxyStatistics(&g, in.ID, ports), "after": galaxyStatistics(next, in.ID, ports), "settings": in.Settings, "revision": next.Revision})
	if _, err = tx.Exec(ctx, `INSERT INTO admin_log(actor,action,data) VALUES($1,'galaxy_update',$2)`, u.ID, audit); err != nil {
		a.internal(w, err)
		return
	}
	a.universeMu.Lock()
	err = tx.Commit(ctx)
	if err == nil {
		a.universe = next
	}
	a.universeMu.Unlock()
	if err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "revision": next.Revision})
}
