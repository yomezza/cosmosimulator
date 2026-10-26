package server

import (
	"encoding/json"
	"testing"
)

func TestGalaxy(t *testing.T) {
	g := generateGalaxy(1000, 42)
	other := generateGalaxy(1000, 42)
	a, _ := json.Marshal(g)
	b, _ := json.Marshal(other)
	if string(a) != string(b) {
		t.Fatal("generation not deterministic")
	}
	if len(g.Stars) != 1000 {
		t.Fatal("wrong count")
	}
	seen := map[[2]int]bool{}
	for _, e := range g.Lanes {
		if e.From == e.To || e.From < 1 || e.To > 1000 {
			t.Fatal("invalid edge")
		}
		key := [2]int{e.From, e.To}
		if seen[key] {
			t.Fatal("duplicate")
		}
		seen[key] = true
		if g.Stars[e.From-1].Region != g.Stars[e.To-1].Region {
			t.Fatal("cross region")
		}
	}
	for _, s := range g.Stars {
		if s.Region >= 0 {
			p := g.path(s.Region+1, s.ID)
			if len(p) == 0 {
				t.Fatal("arm disconnected")
			}
			for i := 1; i < len(p); i++ {
				a, b := p[i-1], p[i]
				if a > b {
					a, b = b, a
				}
				if !seen[[2]int{a, b}] {
					t.Fatal("route jumps across absent edge")
				}
			}
		}
	}
	if g.path(1, 2) != nil || g.path(1, 1000) != nil || g.path(0, 1) != nil {
		t.Fatal("impossible route accepted")
	}
	if len(g.path(1, 1)) != 1 {
		t.Fatal("self route")
	}
	for _, n := range []int{0, 1, 20, 2000} {
		if len(generateGalaxy(n, 1).Stars) != n {
			t.Fatal("count is hardcoded")
		}
	}
}
