package territory

import "testing"

func TestTerritoryOwnershipAndSplit(t *testing.T) {
	connect := func(a, b int) bool { return a-b == 1 || b-a == 1 }
	user := Owner{Kind: "user", ID: 7}
	company := Owner{Kind: "company", ID: 7}
	claims := []Claim{{ID: 3, Sector: 3, Owner: user}, {ID: 1, Sector: 1, Owner: user}}
	if len(Components(claims, connect)) != 2 {
		t.Fatal("disconnected territories merged")
	}
	claims = append(claims, Claim{ID: 2, Sector: 2, Owner: user}, Claim{ID: 4, Sector: 4, Owner: company})
	result := Components(claims, connect)
	if len(result) != 2 || len(result[0]) != 3 || result[0][0].Sector != 1 {
		t.Fatal("bridge or owner partition incorrect", result)
	}
	claims = []Claim{claims[0], claims[1], claims[3]}
	if len(Components(claims, connect)) != 3 {
		t.Fatal("split did not create independent components")
	}
}
