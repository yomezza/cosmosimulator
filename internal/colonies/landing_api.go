package colonies

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/corporations"
	"github.com/yomezza/cosmosimulator/internal/planet/territory"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

type LandingView struct {
	ID         string `json:"id"`
	Sector     int    `json:"sector"`
	Owner      string `json:"owner"`
	Accessible bool   `json:"accessible"`
	Reason     string `json:"reason,omitempty"`
}
type TerritoryView struct {
	Sectors []int    `json:"sectors"`
	Sites   []string `json:"sites"`
	Reason  string   `json:"reason,omitempty"`
}
type LandingState struct {
	Preparing   bool            `json:"preparing"`
	Sites       []LandingView   `json:"sites"`
	Territories []TerritoryView `json:"territories"`
}

// Global overview deliberately omits local coordinates, seeds and other owners'
// private component data. Access is independent of the colonize permission.
func (a *Service) landingState(ctx context.Context, q postgres.DBTX, uid, surface int64) (LandingState, error) {
	out := LandingState{Sites: []LandingView{}, Territories: []TerritoryView{}}
	cid, allowed, err := corporations.CompanyAccess(ctx, q, uid, "planetary.enter")
	if err != nil {
		return out, err
	}
	var components []byte
	err = q.QueryRow(ctx, `SELECT claims_epoch>processed_epoch,components FROM planet_runtime_controls WHERE surface=$1 AND state='enabled'`, surface).Scan(&out.Preparing, &components)
	if err != nil {
		// An uncolonized surface legitimately has no work record.
		if !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		components = []byte("[]")
	}
	permits := func(owner territory.Owner) bool {
		return owner.Kind == "user" && owner.ID == uid || owner.Kind == "company" && owner.ID == cid && allowed
	}
	var groups []landingComponent
	if err = json.Unmarshal(components, &groups); err != nil {
		return out, err
	}
	unavailable := map[string]string{}
	for _, g := range groups {
		for _, id := range g.Unavailable {
			unavailable[id] = g.Reason
		}
	}
	if !out.Preparing {
		for _, g := range groups {
			if permits(g.Owner) {
				out.Territories = append(out.Territories, TerritoryView{Sectors: g.Sectors, Sites: g.Sites, Reason: g.Reason})
			}
		}
	}
	rows, err := q.Query(ctx, `SELECT l.id,l.sector,l.enabled,l.geography_revision=COALESCE(r.revision,0),c.owner_user,c.owner_company,COALESCE(u.username,co.name,'') FROM planet_landing_sites l JOIN planet_colonies c ON c.id=l.colony JOIN planet_surfaces s ON s.id=l.surface LEFT JOIN planet_surface_controls r ON r.star=s.star AND r.planet=s.planet LEFT JOIN users u ON u.id=c.owner_user LEFT JOIN companies co ON co.id=c.owner_company WHERE l.surface=$1 ORDER BY l.sector,l.id`, surface)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var v LandingView
		var enabled, current bool
		var user, company *int64
		if err = rows.Scan(&id, &v.Sector, &enabled, &current, &user, &company, &v.Owner); err != nil {
			return out, err
		}
		v.ID = strconv.FormatInt(id, 10)
		owner := territory.Owner{}
		if user != nil {
			owner = territory.Owner{Kind: "user", ID: *user}
		} else if company != nil {
			owner = territory.Owner{Kind: "company", ID: *company}
		}
		v.Accessible = permits(owner) && enabled && current && !out.Preparing && unavailable[v.ID] == ""
		switch {
		case !permits(owner):
			v.Reason = "Нет права посещения этой территории"
		case unavailable[v.ID] != "":
			v.Reason = unavailable[v.ID]
		case !enabled || !current:
			v.Reason = "Посадочная площадка требует проверки географии"
		case out.Preparing:
			v.Reason = "Подготавливаем посадочную инфраструктуру"
		}
		out.Sites = append(out.Sites, v)
	}
	return out, rows.Err()
}
