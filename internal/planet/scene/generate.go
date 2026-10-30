package scene

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/yomezza/cosmosimulator/internal/geography"
	"github.com/yomezza/cosmosimulator/internal/planet/terrain"
)

func SourceChecksum(g *geography.PlanetSurface) (string, error) {
	b, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Generate reads the supplied snapshot. No new continents or rivers are made.
// seed is derived from the private cosmic seed by the repository, not a client.
func Generate(ctx context.Context, key Key, g *geography.PlanetSurface, seed string) (*Scene, error) {
	if key.Surface <= 0 || key.Sector <= 0 || key.Revision < 0 || len(seed) != 64 || !terrain.Valid(g) {
		return nil, ErrSource
	}
	defs := make(map[int]geography.PlanetSector, len(g.Sectors))
	for _, d := range g.Sectors {
		if d.Number <= 0 || math.IsNaN(d.Center.X+d.Center.Y) || math.IsInf(d.Center.X+d.Center.Y, 0) {
			return nil, ErrSource
		}
		if _, ok := defs[d.Number]; ok {
			return nil, ErrSource
		}
		if len(d.Neighbors) > 6 {
			return nil, ErrSource
		}
		seen := map[int]bool{}
		for _, n := range d.Neighbors {
			if n == d.Number || seen[n] {
				return nil, ErrSource
			}
			seen[n] = true
		}
		defs[d.Number] = d
	}
	def, ok := defs[key.Sector]
	if !ok {
		return nil, ErrSource
	}
	t, err := terrain.Build(ctx, g, def)
	if err != nil {
		return nil, err
	}
	checksum, err := SourceChecksum(g)
	if err != nil {
		return nil, err
	}
	idHash := sha256.Sum256([]byte(fmt.Sprintf("local/%d/%d/%d/%d", key.Surface, key.Sector, key.Revision, GeneratorVersion)))
	s := &Scene{
		meta:  Metadata{Key: key, Generator: GeneratorVersion, Policy: terrain.PolicyVersion, ID: hex.EncodeToString(idHash[:16]), SourceChecksum: checksum, Seed: seed, Biome: def.Biome, Center: def.Center},
		flags: make([]byte, terrain.Size*terrain.Size), heights: make([]uint16, terrain.Size*terrain.Size),
	}
	for y := 0; y < terrain.Size; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < terrain.Size; x++ {
			i := y*terrain.Size + x
			p := t.World(float64(x)+.5, float64(y)+.5)
			h, _ := g.Sample(p.X, p.Y)
			s.heights[i] = uint16(math.Round(math.Max(0, math.Min(65535, h))))
			if terrain.Inside(float64(x)+.5, float64(y)+.5) {
				s.flags[i] |= inside
			}
			if terrain.WaterAt(g, p.X, p.Y) {
				s.flags[i] |= water
			}
			if t.Core[i] {
				s.flags[i] |= core | walkable
			}
		}
	}
	landing, has, err := t.Landing(ctx)
	if err != nil {
		return nil, err
	}
	anchor := -1
	if has {
		s.meta.Landing = &landing
		anchor = landing.SpawnY/terrain.Quantization*terrain.Size + landing.SpawnX/terrain.Quantization
		// Entire visual platform, safety margin and south spawn remain decor-free.
		px, py := float64(landing.X)/terrain.Quantization, float64(landing.Y)/terrain.Quantization
		for y := 0; y < terrain.Size; y++ {
			for x := 0; x < terrain.Size; x++ {
				dx, dy := float64(x)+.5-px, float64(y)+.5-py
				if dx*dx/math.Pow(terrain.LandingRX+2, 2)+dy*dy/math.Pow(terrain.LandingRY+2, 2) <= 1 || math.Abs(dx) <= 3 && dy >= 0 && dy <= terrain.SpawnOffset+2 {
					s.flags[y*terrain.Size+x] |= reserved
				}
			}
		}
	} else {
		for i, v := range t.Core {
			if v {
				anchor = i
				break
			}
		}
	}
	if anchor < 0 {
		return s, nil
	} // All-water sectors have no usable local entry.
	// One BFS tree for every exit; only the already selected dry component is used.
	parent := make([]int, len(s.flags))
	for i := range parent {
		parent[i] = -1
	}
	parent[anchor] = anchor
	queue := []int{anchor}
	for head := 0; head < len(queue); head++ {
		if head%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		i := queue[head]
		x, y := i%terrain.Size, i/terrain.Size
		for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= terrain.Size || ny >= terrain.Size {
				continue
			}
			n := ny*terrain.Size + nx
			if t.Core[n] && parent[n] < 0 {
				parent[n] = i
				queue = append(queue, n)
			}
		}
	}
	neighbors := append([]int(nil), def.Neighbors...)
	sort.Ints(neighbors)
	for _, n := range neighbors {
		d, ok := defs[n]
		if !ok {
			return nil, ErrSource
		}
		other, err := terrain.Build(ctx, g, d)
		if err != nil {
			return nil, err
		}
		p, ok := terrain.Portal(t, other)
		if !ok {
			continue
		}
		dx, dy := d.Center.X-def.Center.X, d.Center.Y-def.Center.Y
		length := math.Hypot(dx, dy)
		q := t.Local(terrain.Point{X: p.X - dx/length*(terrain.Radius+2)/128, Y: p.Y - dy/length*(terrain.Radius+2)/128})
		x, y := int(math.Floor(q.X)), int(math.Floor(q.Y))
		i := y*terrain.Size + x
		if i < 0 || i >= len(parent) || parent[i] < 0 {
			return nil, ErrSource
		}
		s.meta.Portals = append(s.meta.Portals, Portal{Neighbor: n, WorldX: p.X, WorldY: p.Y, Ingress: quant(q)})
		for {
			s.reserve(i%terrain.Size, i/terrain.Size, 2)
			if i == anchor {
				break
			}
			i = parent[i]
		}
	}
	s.reserve(anchor%terrain.Size, anchor/terrain.Size, 3)
	s.decorate(ctx, seed, def.Biome)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Scene) reserve(x, y, r int) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			nx, ny := x+dx, y+dy
			if nx >= 0 && ny >= 0 && nx < terrain.Size && ny < terrain.Size {
				s.flags[ny*terrain.Size+nx] |= reserved
			}
		}
	}
}

// SplitMix64 is part of generator v1; math/rand implementation changes cannot
// reroll a persisted forest. Every attempt consumes the same fixed random slots.
type random uint64

func (r *random) next() uint64 {
	*r += 0x9e3779b97f4a7c15
	v := uint64(*r)
	v = (v ^ (v >> 30)) * 0xbf58476d1ce4e5b9
	v = (v ^ (v >> 27)) * 0x94d049bb133111eb
	return v ^ (v >> 31)
}

type decor struct {
	asset  string
	rx, ry float64
	weight uint64
}

func (s *Scene) decorate(ctx context.Context, seed, biome string) {
	sum := sha256.Sum256([]byte(seed))
	rng := random(binary.LittleEndian.Uint64(sum[:8]))
	// 16 logical px/unit, anchors/footprints taken from the supplied art manifest.
	options := []decor{{"nature/tree_old.png", 1.5, 1.5, 20}, {"nature/tree_young.png", 15.0 / 16, 15.0 / 16, 20}, {"nature/bush_sparse.png", 1, 1, 15}, {"nature/grass_tuft.png", 0, 0, 35}, {"nature/rock_flat.png", 27.0 / 16, 12.0 / 16, 10}}
	if biome == "Сухие равнины" || biome == "Каменистая" || biome == "Ледяная" || biome == "Вулканическая" || biome == "Холодные равнины" {
		options = []decor{{"nature/rock_flat.png", 27.0 / 16, 12.0 / 16, 50}, {"nature/rock_large_cracked.png", 55.0 / 16, 25.0 / 16, 35}, {"nature/grass_dry.png", 0, 0, 15}}
	}
	for slot := 0; slot < 1600; slot++ {
		if slot%64 == 0 && ctx.Err() != nil {
			return
		}
		x, y, choice := int(rng.next()%256), int(rng.next()%256), rng.next()%100
		d := options[0]
		for _, o := range options {
			d = o
			if choice < o.weight {
				break
			}
			choice -= o.weight
		}
		if !s.Walkable(x, y) || s.Reserved(x, y) {
			continue
		}
		obj := Object{ID: fmt.Sprintf("%s-%04x", s.meta.ID, slot), Asset: d.asset, Position: Position{(x * 1024) + 512, (y * 1024) + 512}, RX: int(d.rx * 1024), RY: int(d.ry * 1024)}
		if d.rx > 0 {
			cells := []int{}
			valid := true
			// Conservatively block the ground ellipse's bounding box, inflated by
			// body radius and half-cell diagonal. Never the whole image/canopy.
			rx, ry := d.rx+terrain.Radius+math.Sqrt2/2, d.ry+terrain.Radius+math.Sqrt2/2
			for dy := -int(math.Ceil(ry)); dy <= int(math.Ceil(ry)); dy++ {
				for dx := -int(math.Ceil(rx)); dx <= int(math.Ceil(rx)); dx++ {
					nx, ny := x+dx, y+dy
					if !s.Walkable(nx, ny) || s.Reserved(nx, ny) {
						valid = false
					}
					if nx >= 0 && ny >= 0 && nx < 256 && ny < 256 {
						cells = append(cells, ny*256+nx)
					}
				}
			}
			if !valid {
				continue
			}
			for _, i := range cells {
				s.flags[i] &= ^walkable
			}
		}
		s.meta.Objects = append(s.meta.Objects, obj)
	}
}
