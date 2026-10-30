package scene

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/yomezza/cosmosimulator/internal/geography"
	"github.com/yomezza/cosmosimulator/internal/planet/terrain"
)

func flat() *geography.PlanetSurface {
	g := &geography.PlanetSurface{Width: 6, Height: 4, SeaLevel: 460, GridW: 61, GridH: 41}
	g.Heights = make([]int, g.GridW*g.GridH)
	g.Water = make([]int, len(g.Heights))
	for i := range g.Heights {
		g.Heights[i] = 700
	}
	g.Sectors = []geography.PlanetSector{{Number: 1, Center: geography.SurfacePoint{X: 2, Y: 2}, Neighbors: []int{2}, Biome: "Леса и равнины"}, {Number: 2, Center: geography.SurfacePoint{X: 2 + math.Sqrt(3), Y: 2}, Neighbors: []int{1}, Biome: "Сухие равнины"}}
	return g
}
func makeScene(t *testing.T, g *geography.PlanetSurface, n int) *Scene {
	t.Helper()
	s, err := Generate(context.Background(), Key{Surface: 7, Sector: n, Revision: 3}, g, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSceneSnapshotAndReservedRoutes(t *testing.T) {
	g := flat()
	s := makeScene(t, g, 1)
	again := makeScene(t, g, 1)
	a, hash, err := Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := Encode(again)
	if !bytes.Equal(a, b) {
		t.Fatal("non-deterministic scenery")
	}
	restored, err := Decode(a, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, restored) {
		t.Fatal("snapshot lost geometry or objects")
	}
	m := s.Metadata()
	if m.Landing == nil || len(m.Portals) != 1 || len(m.Objects) < 100 {
		t.Fatal("missing natural scenery/landing/exits")
	}
	// Actual future entry -> every portal remains connected after colliders.
	start := m.Landing.SpawnY/1024*256 + m.Landing.SpawnX/1024
	seen := map[int]bool{start: true}
	queue := []int{start}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		x, y := i%256, i/256
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+d[0], y+d[1]
			n := ny*256 + nx
			if s.Walkable(nx, ny) && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	for _, p := range m.Portals {
		if !seen[p.Ingress.Y/1024*256+p.Ingress.X/1024] {
			t.Fatal("decor blocked reserved exit")
		}
	}
	for _, o := range m.Objects {
		x, y := o.Position.X/1024, o.Position.Y/1024
		if s.Water(x, y) || s.Reserved(x, y) {
			t.Fatal("natural object occupies protected ground")
		}
		if o.RX > 0 && s.Walkable(x, y) {
			t.Fatal("ground collider is missing")
		}
	}
	if err = s.ValidateLanding(*m.Landing); err != nil {
		t.Fatal(err)
	}
	p := *m.Landing
	p.X += 1024
	if !errors.Is(s.ValidateLanding(p), ErrLayout) {
		t.Fatal("moved platform accepted")
	}
	// Public accessors must not let another pin mutate shared geometry/metadata.
	m.Objects[0].ID = "changed"
	m.Landing.X = 0
	m.Portals[0].Neighbor = 999
	if !reflect.DeepEqual(s.Metadata(), restored.Metadata()) {
		t.Fatal("metadata aliases shared scene")
	}
	if s.Walkable(0, 0) || s.Walkable(-1, 128) || s.Walkable(256, 128) {
		t.Fatal("rectangle corner is navigable")
	}
	a[len(a)/2] ^= 1
	if _, err = Decode(a, hash); !errors.Is(err, ErrCorrupt) {
		t.Fatal("corrupt geometry accepted")
	}
	// An unknown version is refused, never silently regenerated.
	restored.meta.Generator = 99
	a, hash, _ = Encode(restored)
	if _, err = Decode(a, hash); !errors.Is(err, ErrVersion) {
		t.Fatal("unknown generator accepted")
	}
	t.Logf("compressed scene=%d bytes, objects=%d, walkable cells=%d", len(b), len(s.meta.Objects), len(seen))
}
func TestSceneCanonicalWaterAndPairedPortals(t *testing.T) {
	g := flat()
	a, b := makeScene(t, g, 1), makeScene(t, g, 2)
	pa, pb := a.Metadata().Portals[0], b.Metadata().Portals[0]
	if pa.WorldX != pb.WorldX || pa.WorldY != pb.WorldY {
		t.Fatal("neighbor side mismatch")
	}
	// River along the common edge must disappear from neither local scene.
	x := (g.Sectors[0].Center.X + g.Sectors[1].Center.X) / 2
	g.Rivers = [][]geography.SurfacePoint{{{X: x, Y: 0}, {X: x, Y: 4}}}
	a, b = makeScene(t, g, 1), makeScene(t, g, 2)
	if len(a.meta.Portals) != 0 || len(b.meta.Portals) != 0 {
		t.Fatal("water crossing treated as dry portal")
	}
	for _, s := range []*Scene{a, b} {
		def := g.Sectors[s.Key().Sector-1]
		tr := terrain.Sector{Definition: def}
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				p := tr.World(float64(x)+.5, float64(y)+.5)
				if s.Water(x, y) != terrain.WaterAt(g, p.X, p.Y) {
					t.Fatal("local shore differs from saved global source")
				}
				h, _ := g.Sample(p.X, p.Y)
				if s.Height(x, y) != uint16(math.Round(h)) {
					t.Fatal("local height differs from global source")
				}
			}
		}
	}
	for i := range g.Heights {
		g.Heights[i] = 100
	}
	g.Rivers = nil
	ocean := makeScene(t, g, 1)
	if ocean.meta.Landing != nil || len(ocean.meta.Objects) != 0 || len(ocean.meta.Portals) != 0 {
		t.Fatal("ocean has local settlement infrastructure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Generate(ctx, Key{7, 1, 3}, g, strings.Repeat("a", 64)); !errors.Is(err, context.Canceled) {
		t.Fatal("generation ignores cancellation", err)
	}
}
