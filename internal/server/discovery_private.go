package server

import (
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	mrand "math/rand"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// Public map coordinates are reproducible; hidden geology uses a key which is
// generated once per database and never leaves the server's private world JSON.
type discoverySource struct {
	key     []byte
	counter uint64
}

func (s *discoverySource) Seed(v int64) { s.counter = uint64(v) }
func (s *discoverySource) Uint64() uint64 {
	h := hmac.New(sha256.New, s.key)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], s.counter)
	s.counter++
	h.Write(b[:])
	return binary.LittleEndian.Uint64(h.Sum(nil))
}
func (s *discoverySource) Int63() int64 { return int64(s.Uint64() >> 1) }
func discoveryRNG(key, domain string) *mrand.Rand {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(domain))
	return mrand.New(&discoverySource{key: h.Sum(nil)})
}
func publicStar(s Star) Star {
	s.DiscoverySeed = ""
	s.ResourceRolls = nil
	s.ResourceWeights = nil
	s.Ports = nil
	return s
}
func publicGalaxies(g *Galaxy) []map[string]any {
	out := []map[string]any{}
	for _, d := range visibleGalaxies(g) {
		out = append(out, map[string]any{"id": d.ID, "name": d.Name, "shape": d.Shape, "opened": d.Opened, "radius": d.Radius, "region_base": d.RegionBase, "territories": d.Territories})
	}
	return out
}

func sealStar(s *Star, secret string, preserve bool) {
	if s.Deleted || s.DiscoverySeed != "" {
		return
	}
	old := *s
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("star/" + strconv.Itoa(s.ID)))
	s.DiscoverySeed = hex.EncodeToString(h.Sum(nil))
	if !preserve {
		s.Planets = nil
		tmp := Galaxy{Stars: []Star{*s}}
		enrichGalaxy(&tmp)
		upgradeGeology(&tmp)
		*s = tmp.Stars[0]
		return
	}
	// Preserve charted bodies and deposits exactly. Secret samples reproduce the
	// current membership but also honor subsequent administrator weight changes.
	s.ResourceRolls = map[string]map[string]float64{}
	save := func(object int, kind string, existing []string) {
		wanted := map[string]bool{}
		for _, id := range existing {
			wanted[id] = true
		}
		samples := map[string]float64{}
		rng := discoveryRNG(s.DiscoverySeed, "preserve/"+strconv.Itoa(object))
		for _, v := range resourceChances(kind) {
			chance := v.chance
			if s.ResourceWeights != nil {
				chance *= float64(s.ResourceWeights[v.id]) / 100
			}
			chance = math.Max(0, math.Min(1, chance))
			r := rng.Float64()
			if wanted[v.id] {
				r *= chance
			} else {
				r = chance + (1-chance)*r
			}
			samples[v.id] = r
		}
		s.ResourceRolls[strconv.Itoa(object)] = samples
	}
	for _, p := range old.Planets {
		save(p.ID, p.Kind, p.Resources)
	}
	save(101, "Пояс астероидов", variedStarResources(old, 101, "Пояс астероидов"))
	if s.ID%2 == 0 {
		save(102, "Ледяное поле", variedStarResources(old, 102, "Ледяное поле"))
	}
}

func secureDiscovery(ctx context.Context, tx pgx.Tx, g *Galaxy) (bool, error) {
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
		sealStar(&g.Stars[i], g.DiscoverySecret, keep[g.Stars[i].ID])
	}
	g.Revision++
	return true, nil
}
