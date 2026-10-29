package universe

func PortalCores(g *Galaxy) map[string][]int {
	m := map[string][]int{}
	for _, d := range g.Catalog {
		if d.Opened {
			m[d.ID] = CoreSystems(g, d.ID)
		}
	}
	return m
}

func BlackHoles(g *Galaxy) map[string]any {
	m := map[string]any{}
	for id, cores := range PortalCores(g) {
		m[id] = map[string]any{"x": 0, "y": 0, "systems": cores}
	}
	return m
}
