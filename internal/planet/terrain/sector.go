// Package terrain reads canonical planetary geography. It never generates shores.
package terrain

import (
	"context"
	"errors"
	"math"

	"github.com/yomezza/cosmosimulator/internal/geography"
)

const Size = 256
const Quantization = 1024
const PolicyVersion = 1
const Radius = 0.625
const RiverHalfWidth = 0.02
const LandingRX = 117.0 / 16
const LandingRY = 66.0 / 16
const SpawnOffset = LandingRY + 3

type Point struct{ X, Y float64 }
type Placement struct{ X, Y, SpawnX, SpawnY int }
type Sector struct {
	Definition geography.PlanetSector
	Surface    *geography.PlanetSurface
	Core       []bool
}

func Valid(g *geography.PlanetSurface) bool {
	return g != nil && g.Width > 0 && g.Height > 0 && !math.IsNaN(g.Width+g.Height) && !math.IsInf(g.Width+g.Height, 0) && g.GridW >= 2 && g.GridH >= 2 && g.GridW <= 4096 && g.GridH <= 4096 && len(g.Heights) == g.GridW*g.GridH && len(g.Water) == len(g.Heights)
}
func Inside(x, y float64) bool {
	dx, dy := math.Abs((x-128)/128), math.Abs((y-128)/128)
	return dx <= math.Sqrt(3)/2 && dy <= 1-dx/math.Sqrt(3)
}
func (s *Sector) World(x, y float64) Point {
	return Point{s.Definition.Center.X + (x-128)/128, s.Definition.Center.Y + (y-128)/128}
}
func (s *Sector) Local(p Point) Point {
	return Point{128 + (p.X-s.Definition.Center.X)*128, 128 + (p.Y-s.Definition.Center.Y)*128}
}
func distance(p, a, b Point) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	den := dx*dx + dy*dy
	t := 0.0
	if den > 0 {
		t = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/den))
	}
	return math.Hypot(p.X-a.X-t*dx, p.Y-a.Y-t*dy)
}

// WaterAt also includes the saved vector rivers used by the overview renderer.
func WaterAt(g *geography.PlanetSurface, x, y float64) bool {
	_, water := g.Sample(x, y)
	if water != 0 {
		return true
	}
	p := Point{x, y}
	for _, river := range g.Rivers {
		for i := 1; i < len(river); i++ {
			a, b := river[i-1], river[i]
			if x < math.Min(a.X, b.X)-RiverHalfWidth || x > math.Max(a.X, b.X)+RiverHalfWidth || y < math.Min(a.Y, b.Y)-RiverHalfWidth || y > math.Max(a.Y, b.Y)+RiverHalfWidth {
				continue
			}
			if distance(p, Point{a.X, a.Y}, Point{b.X, b.Y}) <= RiverHalfWidth {
				return true
			}
		}
	}
	return false
}

func Build(ctx context.Context, g *geography.PlanetSurface, def geography.PlanetSector) (*Sector, error) {
	if !Valid(g) {
		return nil, errors.New("invalid canonical geography")
	}
	s := &Sector{Definition: def, Surface: g, Core: make([]bool, Size*Size)}
	wet := make([]bool, Size*Size)
	for y := 0; y < Size; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < Size; x++ {
			i := y*Size + x
			wet[i] = !Inside(float64(x)+.5, float64(y)+.5)
			if wet[i] {
				continue
			}
			for _, offset := range []Point{{.5, .5}, {0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				p := s.World(float64(x)+offset.X, float64(y)+offset.Y)
				_, w := g.Sample(p.X, p.Y)
				if w != 0 {
					wet[i] = true
					break
				}
			}
		}
	}
	// Rasterize only the bounding box of each saved river segment, not every river
	// for every cell. Include half a cell diagonal for conservative coverage.
	for _, river := range g.Rivers {
		for k := 1; k < len(river); k++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			a, b := s.Local(Point{river[k-1].X, river[k-1].Y}), s.Local(Point{river[k].X, river[k].Y})
			width := RiverHalfWidth*128 + math.Sqrt2/2
			x0, x1 := max(0, int(math.Floor(math.Min(a.X, b.X)-width))), min(Size-1, int(math.Ceil(math.Max(a.X, b.X)+width)))
			y0, y1 := max(0, int(math.Floor(math.Min(a.Y, b.Y)-width))), min(Size-1, int(math.Ceil(math.Max(a.Y, b.Y)+width)))
			for y := y0; y <= y1; y++ {
				for x := x0; x <= x1; x++ {
					if distance(Point{float64(x) + .5, float64(y) + .5}, a, b) <= width {
						wet[y*Size+x] = true
					}
				}
			}
		}
	}
	dry := make([]bool, Size*Size)
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			if wet[y*Size+x] {
				continue
			}
			clear := true
			for dy := -2; dy <= 2 && clear; dy++ {
				for dx := -2; dx <= 2; dx++ {
					nearX, nearY := math.Max(0, math.Abs(float64(dx))-.5), math.Max(0, math.Abs(float64(dy))-.5)
					if math.Hypot(nearX, nearY) > Radius {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= Size || ny >= Size || wet[ny*Size+nx] {
						clear = false
						break
					}
				}
			}
			dry[y*Size+x] = clear
		}
	}
	// The largest dry component anchors the colony; ties use canonical row order.
	visited := make([]bool, Size*Size)
	var best []int
	for i, ok := range dry {
		if !ok || visited[i] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		queue := []int{i}
		visited[i] = true
		for head := 0; head < len(queue); head++ {
			p := queue[head]
			x, y := p%Size, p/Size
			for _, d := range []struct{ x, y int }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d.x, y+d.y
				if nx < 0 || ny < 0 || nx >= Size || ny >= Size {
					continue
				}
				n := ny*Size + nx
				if dry[n] && !visited[n] {
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}
		if len(queue) > len(best) {
			best = queue
		}
	}
	for _, i := range best {
		s.Core[i] = true
	}
	return s, nil
}

func (s *Sector) Contains(p Point) bool {
	x, y := int(math.Floor(p.X)), int(math.Floor(p.Y))
	return x >= 0 && y >= 0 && x < Size && y < Size && s.Core[y*Size+x]
}

func (s *Sector) CanPlace(p Point) bool {
	// Whole platform + safe south-facing spawn corridor must be in the anchor component.
	for y := int(math.Floor(p.Y - LandingRY - Radius)); y <= int(math.Ceil(p.Y+SpawnOffset+1.5)); y++ {
		for x := int(math.Floor(p.X - LandingRX - Radius)); x <= int(math.Ceil(p.X+LandingRX+Radius)); x++ {
			dx, dy := float64(x)+.5-p.X, float64(y)+.5-p.Y
			ellipse := dx*dx/math.Pow(LandingRX+1, 2)+dy*dy/math.Pow(LandingRY+1, 2) <= 1
			corridor := math.Abs(dx) <= 2 && dy >= 0 && dy <= SpawnOffset+1.5
			if (ellipse || corridor) && !s.Contains(Point{float64(x) + .5, float64(y) + .5}) {
				return false
			}
		}
	}
	return s.Contains(p) && s.Contains(Point{p.X, p.Y + SpawnOffset})
}

func (s *Sector) Landing(ctx context.Context) (Placement, bool, error) {
	try := func(dx, dy int) (Placement, bool) {
		p := Point{128.5 + float64(dx), 128.5 + float64(dy)}
		if !s.Contains(p) || !s.CanPlace(p) {
			return Placement{}, false
		}
		return Placement{int(p.X * Quantization), int(p.Y * Quantization), int(p.X * Quantization), int((p.Y + SpawnOffset) * Quantization)}, true
	}
	for r := 0; r <= 128; r++ {
		if err := ctx.Err(); err != nil {
			return Placement{}, false, err
		}
		for dx := -r; dx <= r; dx++ {
			if p, ok := try(dx, -r); ok {
				return p, true, nil
			}
		}
		for dy := -r + 1; dy < r; dy++ {
			for _, dx := range []int{-r, r} {
				if p, ok := try(dx, dy); ok {
					return p, true, nil
				}
			}
		}
		if r > 0 {
			for dx := -r; dx <= r; dx++ {
				if p, ok := try(dx, r); ok {
					return p, true, nil
				}
			}
		}
	}
	return Placement{}, false, nil
}

func adjacent(a, b geography.PlanetSector) bool {
	has := func(s geography.PlanetSector, id int) bool {
		for _, n := range s.Neighbors {
			if n == id {
				return true
			}
		}
		return false
	}
	return has(a, b.Number) && has(b, a.Number)
}

// Portal samples the same shared side for both sectors. Core ingress points
// prevent an isolated island from connecting unrelated exits of one hex.
func Portal(a, b *Sector) (Point, bool) {
	if a != nil && b != nil && a.Definition.Number > b.Definition.Number {
		a, b = b, a
	}
	if a == nil || b == nil || !adjacent(a.Definition, b.Definition) {
		return Point{}, false
	}
	ca, cb := a.Definition.Center, b.Definition.Center
	dx, dy := cb.X-ca.X, cb.Y-ca.Y
	length := math.Hypot(dx, dy)
	if math.Abs(length-math.Sqrt(3)) > .001 {
		return Point{}, false
	}
	nx, ny := dx/length, dy/length
	mid := Point{(ca.X + cb.X) / 2, (ca.Y + cb.Y) / 2}
	ingress := (Radius + 2) / 128
	for i := 3; i < 125; i++ {
		t := (float64(i)+.5)/128 - .5
		p := Point{mid.X - ny*t, mid.Y + nx*t}
		if !a.Contains(a.Local(Point{p.X - nx*ingress, p.Y - ny*ingress})) || !b.Contains(b.Local(Point{p.X + nx*ingress, p.Y + ny*ingress})) {
			continue
		}
		clear := true
		for step := -4; step <= 4 && clear; step++ {
			q := Point{p.X + nx*float64(step)*ingress/4, p.Y + ny*float64(step)*ingress/4}
			for angle := 0; angle < 16; angle++ {
				phi := float64(angle) * math.Pi / 8
				if WaterAt(a.Surface, q.X+math.Cos(phi)*Radius/128, q.Y+math.Sin(phi)*Radius/128) {
					clear = false
					break
				}
			}
		}
		if clear {
			return p, true
		}
	}
	return Point{}, false
}

func Connected(a, b *Sector) bool { _, ok := Portal(a, b); return ok }
