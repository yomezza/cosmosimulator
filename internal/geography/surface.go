package geography

import (
	"container/heap"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/yomezza/cosmosimulator/internal/universe"
)

// Persisted, coarse world-space geography, not rendered pixels. Sector locations
// and future local scenes sample this same lattice and vector river network.
type SurfacePoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type PlanetSurface struct {
	SeaLevel    int              `json:"sea_level"`
	Width       float64          `json:"width"`
	Height      float64          `json:"height"`
	Columns     int              `json:"columns"`
	Rows        int              `json:"rows"`
	GridW       int              `json:"grid_w"`
	GridH       int              `json:"grid_h"`
	Heights     []int            `json:"heights"`
	Water       []int            `json:"water"`
	Rivers      [][]SurfacePoint `json:"rivers"`
	Temperature int              `json:"temperature"`
	Area        float64          `json:"area"`
	Size        string           `json:"size"`
	Sectors     []PlanetSector   `json:"sectors,omitempty"`
}

type PlanetSector struct {
	Number    int               `json:"number"`
	Column    int               `json:"column"`
	Row       int               `json:"row"`
	Center    SurfacePoint      `json:"center"`
	Neighbors []int             `json:"neighbors"`
	Land      float64           `json:"land"`
	Type      string            `json:"type"`
	Biome     string            `json:"biome"`
	Area      float64           `json:"area"`
	Estimates map[string]string `json:"estimates"`
}

func planetRadius(p universe.Planet) float64 {
	if p.Gravity > 0 {
		return math.Sqrt(p.Mass / p.Gravity)
	}
	return math.Pow(math.Max(.01, p.Mass), .27)
}

func PlanetSize(p universe.Planet) int {
	r := planetRadius(p)
	switch {
	case r < .8:
		return 0
	case r < 1.1:
		return 1
	case r < 1.4:
		return 2
	default:
		return 3
	}
}

func SurfaceSeed(g *universe.Galaxy, s universe.Star, p universe.Planet) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/surface-v1/%d/%d/%d/%s", g.DiscoverySecret, g.Seed, s.ID, p.ID, s.DiscoverySeed)))
	return hex.EncodeToString(sum[:])
}

func noiseHash(seed uint64, x, y int) float64 {
	v := seed + uint64(x)*0x9e3779b97f4a7c15 + uint64(y)*0xbf58476d1ce4e5b9
	v = (v ^ (v >> 30)) * 0xbf58476d1ce4e5b9
	v = (v ^ (v >> 27)) * 0x94d049bb133111eb
	return float64(v^(v>>31)) / float64(^uint64(0))
}

func surfaceNoise(seed uint64, x, y float64) float64 {
	ix, iy := int(math.Floor(x)), int(math.Floor(y))
	a, b := x-float64(ix), y-float64(iy)
	a = a * a * (3 - 2*a)
	b = b * b * (3 - 2*b)
	return (noiseHash(seed, ix, iy)*(1-a)+noiseHash(seed, ix+1, iy)*a)*(1-b) + (noiseHash(seed, ix, iy+1)*(1-a)+noiseHash(seed, ix+1, iy+1)*a)*b
}

type floodCell struct {
	index  int
	height float64
}

type floodQueue []floodCell

func (q floodQueue) Len() int { return len(q) }

func (q floodQueue) Less(i, j int) bool {
	if q[i].height == q[j].height {
		return q[i].index < q[j].index
	}
	return q[i].height < q[j].height
}

func (q floodQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *floodQueue) Push(x any) { *q = append(*q, x.(floodCell)) }

func (q *floodQueue) Pop() any { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

func Generate(p universe.Planet, seedText string, count int) PlanetSurface {
	sum := sha256.Sum256([]byte(seedText))
	seed := binary.LittleEndian.Uint64(sum[:8])
	cols := 10
	if count == 160 {
		cols = 16
	}
	if count == 200 {
		cols = 20
	}
	rows := count / cols
	out := PlanetSurface{Width: math.Sqrt(3) * (float64(cols) + .5), Height: 2 + 1.5*float64(rows-1), Columns: cols, Rows: rows, GridW: 129, GridH: 97, Temperature: p.Temperature - 273, Area: 510072000 * math.Pow(planetRadius(p), 2), Size: []string{"Маленькая", "Средняя", "Большая", "Гигантская"}[PlanetSize(p)], Rivers: [][]SurfacePoint{}}
	wet := p.Kind == "Садовый мир" || p.Kind == "Ледяная"
	out.SeaLevel = -1
	if wet {
		out.SeaLevel = 460
	}
	n := out.GridW * out.GridH
	out.Heights = make([]int, n)
	out.Water = make([]int, n)
	parents := make([]int, n)
	seen := make([]bool, n)
	queue := &floodQueue{}
	heap.Init(queue)
	adj := func(i int) []int {
		x, y := i%out.GridW, i/out.GridW
		a := []int{}
		for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {-1, 1}, {1, -1}, {1, 1}} {
			xx, yy := x+d[0], y+d[1]
			if xx >= 0 && yy >= 0 && xx < out.GridW && yy < out.GridH {
				a = append(a, yy*out.GridW+xx)
			}
		}
		return a
	}
	for i := 0; i < n; i++ {
		x, y := float64(i%out.GridW)/float64(out.GridW-1), float64(i/out.GridW)/float64(out.GridH-1)
		h := .52*surfaceNoise(seed, x*3, y*3) + .26*surfaceNoise(seed+1, x*7, y*7) + .14*surfaceNoise(seed+2, x*15, y*15) + .08*surfaceNoise(seed+3, x*31, y*31)
		h -= .12 * math.Pow(math.Max(math.Abs(x-.5), math.Abs(y-.5))*2, 4)
		out.Heights[i] = int(h * 1000)
		if wet && h < .46 {
			out.Water[i] = 1
		}
		parents[i] = -1
		if out.Water[i] == 1 || i%out.GridW == 0 || i%out.GridW == out.GridW-1 || i < out.GridW || i >= n-out.GridW {
			seen[i] = true
			heap.Push(queue, floodCell{i, float64(out.Heights[i])})
		}
	}
	// Priority-flood connects depressions to drainage; lakes occupy filled lowlands.
	for queue.Len() > 0 {
		c := heap.Pop(queue).(floodCell)
		for _, next := range adj(c.index) {
			if seen[next] {
				continue
			}
			seen[next] = true
			parents[next] = c.index
			h := float64(out.Heights[next])
			if wet && c.height-h > 25 && out.Water[next] == 0 {
				out.Water[next] = 2
			}
			heap.Push(queue, floodCell{next, math.Max(h, c.height)})
		}
	}
	point := func(i int) SurfacePoint {
		return SurfacePoint{float64(i%out.GridW) / float64(out.GridW-1) * out.Width, float64(i/out.GridW) / float64(out.GridH-1) * out.Height}
	}
	for source := 0; source < 9 && p.Kind == "Садовый мир"; source++ {
		start := -1
		score := -1.0
		for i := 0; i < n; i++ {
			if out.Water[i] > 0 || out.Heights[i] < 560 {
				continue
			}
			v := float64(out.Heights[i]) + noiseHash(seed+uint64(source+10), i, 0)*100
			if v > score {
				score = v
				start = i
			}
		}
		if start < 0 {
			continue
		}
		path := []SurfacePoint{}
		for i, step := start, 0; i >= 0 && step < n; i, step = parents[i], step+1 {
			path = append(path, point(i))
			if out.Water[i] > 0 {
				break
			}
		}
		if len(path) > 4 {
			out.Rivers = append(out.Rivers, smoothRiver(path))
			for _, point := range path {
				index := out.index(point.X, point.Y)
				if out.Water[index] == 0 {
					out.Water[index] = 3
				}
			}
		}
	}
	// Global land components distinguish detached islands from continental coasts.
	component := make([]int, n)
	sizes := []int{0}
	for i := 0; i < n; i++ {
		if out.Water[i] > 0 || component[i] != 0 {
			continue
		}
		id := len(sizes)
		todo := []int{i}
		component[i] = id
		size := 0
		for len(todo) > 0 {
			v := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			size++
			for _, next := range adj(v) {
				if out.Water[next] == 0 && component[next] == 0 {
					component[next] = id
					todo = append(todo, next)
				}
			}
		}
		sizes = append(sizes, size)
	}
	largest := 0
	for _, size := range sizes {
		largest = max(largest, size)
	}
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			s := PlanetSector{Number: row*cols + col + 1, Column: col, Row: row, Center: SurfacePoint{math.Sqrt(3) * (.5 + float64(col) + .5*float64(row%2)), 1 + 1.5*float64(row)}, Neighbors: []int{}, Area: out.Area / float64(count), Estimates: map[string]string{}}
			land, total, island := 0, 0, 0
			for dy := -1.0; dy <= 1; dy += .08 {
				for dx := -.87; dx <= .87; dx += .08 {
					if !inPlanetHex(dx, dy) {
						continue
					}
					i := out.index(s.Center.X+dx, s.Center.Y+dy)
					total++
					if _, water := out.Sample(s.Center.X+dx, s.Center.Y+dy); water == 0 {
						land++
						if sizes[component[i]] < largest/3 {
							island++
						}
					}
				}
			}
			s.Land = math.Floor(10000*float64(land)/float64(total)) / 100
			s.Type = "Континентальный"
			if s.Land < 10 {
				s.Type = "Океанический"
			} else if island*2 > land {
				s.Type = "Островной"
			} else if s.Land < 90 {
				s.Type = "Прибрежный"
			}
			s.Biome = "Леса и равнины"
			if p.Kind != "Садовый мир" {
				s.Biome = p.Kind
			} else if out.Temperature < 0 {
				s.Biome = "Холодные равнины"
			} else if noiseHash(seed, col, row) > .7 {
				s.Biome = "Сухие равнины"
			}
			for _, id := range p.Resources {
				s.Estimates[id] = []string{"Бедные залежи", "Умеренные залежи", "Богатые залежи", "Очень богатые залежи"}[int(noiseHash(seed+19, col+len(id), row)*4)%4]
			}
			out.Sectors = append(out.Sectors, s)
		}
	}
	for i := range out.Sectors {
		for j := range out.Sectors {
			if i != j && math.Hypot(out.Sectors[i].Center.X-out.Sectors[j].Center.X, out.Sectors[i].Center.Y-out.Sectors[j].Center.Y) < 1.74 {
				out.Sectors[i].Neighbors = append(out.Sectors[i].Neighbors, j+1)
			}
		}
	}
	return out
}

func inPlanetHex(x, y float64) bool {
	return math.Abs(x) <= math.Sqrt(3)/2 && math.Abs(y) <= 1-math.Abs(x)/math.Sqrt(3)
}

func (s PlanetSurface) index(x, y float64) int {
	ix := max(0, min(s.GridW-1, int(math.Round(x/s.Width*float64(s.GridW-1)))))
	iy := max(0, min(s.GridH-1, int(math.Round(y/s.Height*float64(s.GridH-1)))))
	return iy*s.GridW + ix
}

// Canonical interpolation for overview and future per-sector scenes. Coordinates
// are in planet space; adjacent sectors therefore sample identical boundaries.
// Sample reads the canonical world-space terrain lattice and river network.
// Sector runtimes must use this saved geography instead of generating new shores.
func (s PlanetSurface) Sample(x, y float64) (height float64, water int) {
	gx := math.Max(0, math.Min(float64(s.GridW-1), x/s.Width*float64(s.GridW-1)))
	gy := math.Max(0, math.Min(float64(s.GridH-1), y/s.Height*float64(s.GridH-1)))
	ix, iy := int(gx), int(gy)
	fx, fy := gx-float64(ix), gy-float64(iy)
	ids := []int{iy*s.GridW + ix, iy*s.GridW + min(ix+1, s.GridW-1), min(iy+1, s.GridH-1)*s.GridW + ix, min(iy+1, s.GridH-1)*s.GridW + min(ix+1, s.GridW-1)}
	weights := []float64{(1 - fx) * (1 - fy), fx * (1 - fy), (1 - fx) * fy, fx * fy}
	fresh := 0.0
	for i, id := range ids {
		height += float64(s.Heights[id]) * weights[i]
		if s.Water[id] >= 2 {
			fresh += weights[i]
		}
	}
	if height < float64(s.SeaLevel) {
		water = 1
	} else if fresh > .5 {
		water = 2
	}
	return
}

// Interpolating curves retain drainage junctions and are persisted as world-space
// polylines, so overview and future local scenes share precisely the same rivers.
func smoothRiver(path []SurfacePoint) []SurfacePoint {
	out := []SurfacePoint{path[0]}
	for i := 0; i < len(path)-1; i++ {
		a, b, c, d := path[max(0, i-1)], path[i], path[i+1], path[min(len(path)-1, i+2)]
		for step := 1; step <= 4; step++ {
			t := float64(step) / 4
			t2, t3 := t*t, t*t*t
			blend := func(a, b, c, d float64) float64 {
				return .5 * ((2 * b) + (-a+c)*t + (2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
			}
			out = append(out, SurfacePoint{blend(a.X, b.X, c.X, d.X), blend(a.Y, b.Y, c.Y, d.Y)})
		}
	}
	return out
}
