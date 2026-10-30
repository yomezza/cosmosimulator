// Package pathing implements bounded, deterministic navigation on inflated masks.
package pathing

import (
	"container/heap"
	"context"
	"errors"
	"math"
)

const Size = 256
const Q = 1024

var ErrBlocked = errors.New("target_blocked")
var ErrUnreachable = errors.New("target_unreachable")
var ErrBudget = errors.New("path_budget_exceeded")

type Grid interface{ Walkable(int, int) bool }
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func FromQ(x, y int) Point            { return Point{float64(x) / Q, float64(y) / Q} }
func (p Point) Quantized() (int, int) { return int(math.Round(p.X * Q)), int(math.Round(p.Y * Q)) }
func Valid(p Point) bool {
	return !math.IsNaN(p.X+p.Y) && !math.IsInf(p.X+p.Y, 0) && p.X >= 0 && p.Y >= 0 && p.X < Size && p.Y < Size
}
func Clear(g Grid, p Point) bool { return Valid(p) && g.Walkable(int(p.X), int(p.Y)) }

// Segment uses a supercover DDA. At a grid corner BOTH side cells must be free.
// The supplied mask already includes the character body and object ground bases.
func Segment(g Grid, a, b Point) bool {
	if !Clear(g, a) || !Clear(g, b) {
		return false
	}
	x, y, ex, ey := int(a.X), int(a.Y), int(b.X), int(b.Y)
	dx, dy := b.X-a.X, b.Y-a.Y
	sx, sy := 1, 1
	if dx < 0 {
		sx = -1
	}
	if dy < 0 {
		sy = -1
	}
	tx, ty, stepx, stepy := math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(1)
	if dx != 0 {
		edge := float64(x + 1)
		if sx < 0 {
			edge = float64(x)
		}
		tx = (edge - a.X) / dx
		stepx = math.Abs(1 / dx)
	}
	if dy != 0 {
		edge := float64(y + 1)
		if sy < 0 {
			edge = float64(y)
		}
		ty = (edge - a.Y) / dy
		stepy = math.Abs(1 / dy)
	}
	for n := 0; n < Size*2+2; n++ {
		if x == ex && y == ey {
			return true
		}
		if math.Abs(tx-ty) < 1e-10 {
			if !g.Walkable(x+sx, y) || !g.Walkable(x, y+sy) {
				return false
			}
			x += sx
			y += sy
			tx += stepx
			ty += stepy
		} else if tx < ty {
			x += sx
			tx += stepx
		} else {
			y += sy
			ty += stepy
		}
		if !g.Walkable(x, y) {
			return false
		}
	}
	return false
}

type node struct{ i, cost, score, order int }
type queue []node

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].score != q[j].score {
		return q[i].score < q[j].score
	}
	return q[i].order < q[j].order
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(v any)   { *q = append(*q, v.(node)) }
func (q *queue) Pop() any     { v := (*q)[len(*q)-1]; *q = (*q)[:len(*q)-1]; return v }
func heuristic(a, b int) int {
	dx, dy := abs(a%Size-b%Size), abs(a/Size-b/Size)
	return 10*max(dx, dy) + 4*min(dx, dy)
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Target selects a cell, not an arbitrary nearest free point. A blocked selected
// cell is refused. Its centre is the explicit normalized goal in path.accepted.
func Find(ctx context.Context, g Grid, start, target Point, budget int) ([]Point, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !Clear(g, start) || !Clear(g, target) {
		return nil, ErrBlocked
	}
	if budget < 1 {
		return nil, ErrBudget
	}
	end := int(target.Y)*Size + int(target.X)
	begin := int(start.Y)*Size + int(start.X)
	goal := Point{float64(end%Size) + .5, float64(end/Size) + .5}
	if Segment(g, start, goal) {
		return []Point{start, goal}, nil
	}
	costs := make([]int, Size*Size)
	parent := make([]int, Size*Size)
	for i := range costs {
		costs[i] = math.MaxInt32
		parent[i] = -1
	}
	costs[begin] = 0
	open := &queue{{i: begin, score: heuristic(begin, end)}}
	heap.Init(open)
	order, expanded := 0, 0
	found := false
	for open.Len() > 0 {
		if expanded%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		n := heap.Pop(open).(node)
		if n.cost != costs[n.i] {
			continue
		}
		expanded++
		if expanded > budget {
			return nil, ErrBudget
		}
		if n.i == end {
			found = true
			break
		}
		x, y := n.i%Size, n.i/Size
		for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}, {1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= Size || ny >= Size || !g.Walkable(nx, ny) {
				continue
			}
			weight := 10
			if d[0] != 0 && d[1] != 0 {
				if !g.Walkable(nx, y) || !g.Walkable(x, ny) {
					continue
				}
				weight = 14
			}
			i := ny*Size + nx
			c := n.cost + weight
			if c >= costs[i] {
				continue
			}
			costs[i] = c
			parent[i] = n.i
			order++
			heap.Push(open, node{i, c, c + heuristic(i, end), order})
		}
	}
	if !found {
		return nil, ErrUnreachable
	}
	raw := []Point{}
	for i := end; i != begin; i = parent[i] {
		if i < 0 || len(raw) > 2048 {
			return nil, ErrBudget
		}
		raw = append(raw, Point{float64(i%Size) + .5, float64(i/Size) + .5})
	}
	raw = append(raw, Point{float64(begin%Size) + .5, float64(begin/Size) + .5})
	for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
		raw[i], raw[j] = raw[j], raw[i]
	}
	if !Segment(g, start, raw[0]) {
		return nil, ErrBlocked
	}
	raw = append([]Point{start}, raw...)
	path := []Point{start}
	checks := 0
	for at := 0; at < len(raw)-1; {
		next := at + 1
		for j := len(raw) - 1; j > at+1; j-- {
			checks++
			if checks > 4096 {
				return nil, ErrBudget
			}
			if checks%64 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			if Segment(g, raw[at], raw[j]) {
				next = j
				break
			}
		}
		if !Segment(g, raw[at], raw[next]) {
			return nil, ErrBlocked
		}
		path = append(path, raw[next])
		at = next
	}
	return path, nil
}
