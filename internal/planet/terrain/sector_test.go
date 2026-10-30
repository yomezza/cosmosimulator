package terrain

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/yomezza/cosmosimulator/internal/geography"
)

func flatSurface() geography.PlanetSurface {
	g := geography.PlanetSurface{SeaLevel: 460, Width: 10, Height: 4, GridW: 101, GridH: 41}
	g.Heights = make([]int, g.GridW*g.GridH)
	g.Water = make([]int, len(g.Heights))
	for i := range g.Heights {
		g.Heights[i] = 700
	}
	for i := 0; i < 3; i++ {
		g.Sectors = append(g.Sectors, geography.PlanetSector{Number: i + 1, Center: geography.SurfacePoint{X: 1 + float64(i)*math.Sqrt(3), Y: 2}})
	}
	g.Sectors[0].Neighbors = []int{2}
	g.Sectors[1].Neighbors = []int{1, 3}
	g.Sectors[2].Neighbors = []int{2}
	return g
}

func TestCanonicalDryConnectivity(t *testing.T) {
	ctx := context.Background()
	g := flatSurface()
	a, err := Build(ctx, &g, g.Sectors[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build(ctx, &g, g.Sectors[1])
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Build(ctx, &g, g.Sectors[0])
	if !reflect.DeepEqual(a.Core, again.Core) {
		t.Fatal("unstable dry component")
	}
	pab, ok1 := Portal(a, b)
	pba, ok2 := Portal(b, a)
	if !ok1 || !ok2 || pab != pba {
		t.Fatal("shared portal differs across side order")
	}
	if !Connected(a, b) || !Connected(b, a) {
		t.Fatal("shared dry edge must connect symmetrically")
	}
	p, ok, err := a.Landing(ctx)
	if err != nil || !ok {
		t.Fatal("no landing on dry land", err)
	}
	p2, ok, _ := again.Landing(ctx)
	if !ok || p != p2 {
		t.Fatal("unstable placement")
	}
	if !a.CanPlace(Point{float64(p.X) / Quantization, float64(p.Y) / Quantization}) || !a.Contains(Point{float64(p.SpawnX) / Quantization, float64(p.SpawnY) / Quantization}) {
		t.Fatal("unsafe landing/spawn")
	}
	if a.CanPlace(Point{0, 0}) {
		t.Fatal("landing outside hex")
	}
	// A saved vector river is authoritative even if the raster has no fresh-water cells.
	x := (g.Sectors[0].Center.X + g.Sectors[1].Center.X) / 2
	g.Rivers = [][]geography.SurfacePoint{{{X: x, Y: 0}, {X: x, Y: 4}}}
	a, _ = Build(ctx, &g, g.Sectors[0])
	b, _ = Build(ctx, &g, g.Sectors[1])
	if Connected(a, b) || !WaterAt(&g, x, 2) {
		t.Fatal("river crossing incorrectly allowed")
	}
	for i := range g.Heights {
		g.Heights[i] = 100
	}
	ocean, _ := Build(ctx, &g, g.Sectors[0])
	_, ok, err = ocean.Landing(ctx)
	if err != nil || ok {
		t.Fatal("landing placed in ocean", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = Build(cancelled, &g, g.Sectors[0]); err == nil {
		t.Fatal("cancelled work continued")
	}
}

func TestIslandsCannotBridgeOppositeEdges(t *testing.T) {
	g := flatSurface()
	x := g.Sectors[1].Center.X
	g.Rivers = [][]geography.SurfacePoint{{{X: x, Y: 0}, {X: x, Y: 4}}}
	a, _ := Build(context.Background(), &g, g.Sectors[0])
	b, _ := Build(context.Background(), &g, g.Sectors[1])
	c, _ := Build(context.Background(), &g, g.Sectors[2])
	if Connected(a, b) && Connected(b, c) {
		t.Fatal("different islands in the middle hex created a false bridge")
	}
}
