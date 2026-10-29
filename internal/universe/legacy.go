package universe

import "sync"

// Legacy station membership is computed only when the cosmic station policy asks
// for it. Importing geography for a sector server must not generate a galaxy.
var originalCore = sync.OnceValue(func() map[int]bool {
	g := Generate(1000, 20261026)
	m := map[int]bool{}
	for _, id := range CoreSystems(&g) {
		m[id] = true
	}
	return m
})
