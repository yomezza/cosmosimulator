package access

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/yomezza/cosmosimulator/internal/planet/scene"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

type Territory struct {
	Colony int64
	Owner  string
}

// Sector authorizes an actual completed colony, including sectors without a
// landing module. The original ship/site reservation remains a separate gate.
func Sector(ctx context.Context, q postgres.Querier, uid int64, k scene.Key) (Territory, error) {
	var t Territory
	err := q.QueryRow(ctx, `SELECT cl.id,COALESCE(co.name,'') FROM planet_colonies cl
 JOIN planet_projects p ON p.id=cl.project JOIN planet_surfaces s ON s.id=cl.surface
 JOIN planet_runtime_controls rc ON rc.surface=s.id
 LEFT JOIN planet_surface_controls sc ON sc.star=s.star AND sc.planet=s.planet
 JOIN users u ON u.id=$1 LEFT JOIN companies co ON co.id=cl.owner_company
 LEFT JOIN company_roles r ON r.id=u.company_role AND r.company_id=u.company_id
 WHERE cl.surface=$2 AND cl.sector=$3 AND p.status='complete' AND rc.state='enabled'
 AND rc.geography_revision=$4 AND COALESCE(sc.revision,0)=$4 AND NOT COALESCE(sc.deleted,false)
 AND (cl.owner_user=$1 OR(cl.owner_company=u.company_id AND(co.owner_id=$1 OR COALESCE(r.permissions,'[]'::jsonb)?'planetary.enter')))`, uid, k.Surface, k.Sector, k.Revision).Scan(&t.Colony, &t.Owner)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrDenied
	}
	if t.Owner == "" {
		t.Owner = "Личная территория"
	}
	return t, err
}
