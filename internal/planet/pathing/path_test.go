package pathing

import (
	"context"
	"errors"
	"math"
	"testing"
)

type grid map[[2]int]bool

func (g grid) Walkable(x, y int) bool {
	return x >= 0 && y >= 0 && x < Size && y < Size && !g[[2]int{x, y}]
}
func TestBoundedPathsAndCorners(t *testing.T) {
	g := grid{}
	for y := 0; y < 256; y++ {
		if y != 8 {
			g[[2]int{8, y}] = true
		}
	}
	start, goal := Point{2.5, 3.5}, Point{14.2, 3.9}
	path, err := Find(context.Background(), g, start, goal, 24000)
	if err != nil {
		t.Fatal(err)
	}
	if path[0] != start || path[len(path)-1] != (Point{14.5, 3.5}) {
		t.Fatal("wrong normalized selected cell", path)
	}
	for i := 1; i < len(path); i++ {
		if !Segment(g, path[i-1], path[i]) {
			t.Fatal("path crossed a wall")
		}
	}
	if Segment(g, start, Point{14.5, 3.5}) {
		t.Fatal("wall disappeared in smoothing")
	}
	sealed := grid{{1, 0}: true, {0, 1}: true}
	if Segment(sealed, Point{.5, .5}, Point{1.5, 1.5}) {
		t.Fatal("diagonal corner cutting")
	}
	if _, err = Find(context.Background(), sealed, Point{.5, .5}, Point{1.5, 1.5}, 100); !errors.Is(err, ErrUnreachable) {
		t.Fatal(err)
	}
	if _, err = Find(context.Background(), g, start, Point{8.5, 1.5}, 100); !errors.Is(err, ErrBlocked) {
		t.Fatal("blocked goal relocated", err)
	}
	if _, err = Find(context.Background(), g, start, goal, 1); !errors.Is(err, ErrBudget) {
		t.Fatal("unbounded search", err)
	}
	for _, p := range []Point{{-1, 2}, {256, 2}, {math.NaN(), 2}, {2, math.Inf(1)}} {
		if _, err = Find(context.Background(), g, start, p, 100); !errors.Is(err, ErrBlocked) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Find(ctx, grid{}, start, goal, 100); !errors.Is(err, context.Canceled) {
		t.Fatal("path ignored cancellation", err)
	}
}
