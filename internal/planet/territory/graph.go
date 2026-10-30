// Package territory groups canonical colonies without storing another owner model.
package territory

import "sort"

type Owner struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}
type Claim struct {
	ID     int64
	Sector int
	Owner  Owner
}

// Components is deterministic and never merges user/company owners of equal IDs.
func Components(claims []Claim, connect func(int, int) bool) [][]Claim {
	sorted := append([]Claim(nil), claims...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Sector < sorted[j].Sector })
	seen := make([]bool, len(sorted))
	out := [][]Claim{}
	for i, c := range sorted {
		if seen[i] {
			continue
		}
		seen[i] = true
		component := []Claim{c}
		indices := []int{i}
		for head := 0; head < len(indices); head++ {
			at := indices[head]
			for j, other := range sorted {
				if !seen[j] && sorted[at].Owner == other.Owner && connect(sorted[at].Sector, other.Sector) {
					seen[j] = true
					indices = append(indices, j)
					component = append(component, other)
				}
			}
		}
		sort.Slice(component, func(i, j int) bool { return component[i].Sector < component[j].Sector })
		out = append(out, component)
	}
	return out
}
