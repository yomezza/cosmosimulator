package colonies

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/cosmosimulator/internal/geography"
	"github.com/yomezza/cosmosimulator/internal/planet/terrain"
	"github.com/yomezza/cosmosimulator/internal/planet/territory"
)

type landingWorkError struct {
	Surface int64
	Cause   error
}

func (e *landingWorkError) Error() string { return "landing reconciliation failed" }
func (e *landingWorkError) Unwrap() error { return e.Cause }

type landingSite struct {
	ID        int64
	Colony    int64
	Sector    int
	Revision  int64
	Enabled   bool
	Placement terrain.Placement
}
type landingComponent struct {
	Owner       territory.Owner `json:"owner"`
	Sectors     []int           `json:"sectors"`
	Sites       []string        `json:"sites"`
	Reason      string          `json:"reason,omitempty"`
	Unavailable []string        `json:"unavailable,omitempty"`
}
type landingSnapshot struct {
	Surface, Revision, ClaimsEpoch, LayoutEpoch int64
	Star, Planet                                int
	Name                                        string
	Geography                                   geography.PlanetSurface
	Claims                                      []territory.Claim
	Sites                                       []landingSite
}

// Read a short consistent snapshot. No terrain calculation runs inside this tx.
func (a *Service) landingSnapshot(ctx context.Context) (*landingSnapshot, error) {
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	var s landingSnapshot
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT c.surface,c.geography_revision,c.claims_epoch,c.layout_epoch,s.star,s.planet,s.name,s.data FROM planet_runtime_controls c JOIN planet_surfaces s ON s.id=c.surface WHERE c.state='enabled' AND c.retry_at<=now() AND (c.claims_epoch>c.processed_epoch OR c.issues<>'[]'::jsonb) ORDER BY c.retry_at,c.surface LIMIT 1`).Scan(&s.Surface, &s.Revision, &s.ClaimsEpoch, &s.LayoutEpoch, &s.Star, &s.Planet, &s.Name, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &s.Geography); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.sector,c.owner_user,c.owner_company FROM planet_colonies c JOIN planet_projects p ON p.id=c.project WHERE c.surface=$1 AND p.status='complete' ORDER BY c.sector`, s.Surface)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c territory.Claim
		var user, company *int64
		if err = rows.Scan(&c.ID, &c.Sector, &user, &company); err != nil {
			break
		}
		if user != nil {
			c.Owner = territory.Owner{Kind: "user", ID: *user}
		} else if company != nil {
			c.Owner = territory.Owner{Kind: "company", ID: *company}
		}
		s.Claims = append(s.Claims, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT id,colony,sector,geography_revision,enabled,x_q,y_q,spawn_x_q,spawn_y_q FROM planet_landing_sites WHERE surface=$1 ORDER BY id`, s.Surface)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var site landingSite
		if err = rows.Scan(&site.ID, &site.Colony, &site.Sector, &site.Revision, &site.Enabled, &site.Placement.X, &site.Placement.Y, &site.Placement.SpawnX, &site.Placement.SpawnY); err != nil {
			break
		}
		s.Sites = append(s.Sites, site)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s, nil
}

// ReconcileLandingSites computes one surface outside world locks and commits only
// if claims/revision/layout epochs still match. Safe under multiple cosmic workers.
func (a *Service) ReconcileLandingSites(ctx context.Context) (applied bool, failure error) {
	snapshot, err := a.landingSnapshot(ctx)
	if err != nil || snapshot == nil {
		return false, err
	}
	s := snapshot
	defer func() {
		if failure != nil {
			failure = &landingWorkError{Surface: s.Surface, Cause: failure}
		}
	}()
	scenes := map[int]*terrain.Sector{}
	definitions := map[int]geography.PlanetSector{}
	for _, def := range s.Geography.Sectors {
		definitions[def.Number] = def
	}
	for _, c := range s.Claims {
		def, ok := definitions[c.Sector]
		if !ok {
			return false, errors.New("colony references missing canonical sector")
		}
		scene, e := terrain.Build(ctx, &s.Geography, def)
		if e != nil {
			return false, e
		}
		scenes[c.Sector] = scene
	}
	components := territory.Components(s.Claims, func(x, y int) bool { return terrain.Connected(scenes[x], scenes[y]) })
	type proposed struct {
		Claim     territory.Claim
		Placement terrain.Placement
		Component int
	}
	var proposals []proposed
	results := make([]landingComponent, 0, len(components))
	issues := make([]landingComponent, 0)
	occupied := map[int64]bool{}
	for _, site := range s.Sites {
		occupied[site.Colony] = true
	}
	for _, group := range components {
		result := landingComponent{Owner: group[0].Owner, Sectors: []int{}, Sites: []string{}}
		for _, c := range group {
			result.Sectors = append(result.Sectors, c.Sector)
			for _, site := range s.Sites {
				if site.Colony == c.ID && site.Enabled {
					scene := scenes[c.Sector]
					p := terrain.Point{X: float64(site.Placement.X) / terrain.Quantization, Y: float64(site.Placement.Y) / terrain.Quantization}
					if site.Revision != s.Revision || !scene.CanPlace(p) {
						result.Reason = "Посадочная площадка требует проверки географии"
						result.Unavailable = append(result.Unavailable, strconv.FormatInt(site.ID, 10))
						continue
					}
					result.Sites = append(result.Sites, strconv.FormatInt(site.ID, 10))
				}
			}
		}
		if len(result.Sites) == 0 && result.Reason == "" {
			for _, c := range group {
				if occupied[c.ID] {
					continue
				}
				p, ok, e := scenes[c.Sector].Landing(ctx)
				if e != nil {
					return false, e
				}
				if ok {
					proposals = append(proposals, proposed{c, p, len(results)})
					break
				}
			}
			if len(proposals) == 0 || proposals[len(proposals)-1].Component != len(results) {
				result.Reason = "Нет безопасного места для посадочной площадки"
			}
		}
		if result.Reason != "" {
			issues = append(issues, result)
		}
		results = append(results, result)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		return false, err
	}
	var claims, layout, revision int64
	err = tx.QueryRow(ctx, `SELECT c.claims_epoch,c.layout_epoch,COALESCE(r.revision,0) FROM planet_runtime_controls c JOIN planet_surfaces s ON s.id=c.surface LEFT JOIN planet_surface_controls r ON r.star=s.star AND r.planet=s.planet WHERE c.surface=$1 AND c.state='enabled' FOR UPDATE OF c`, s.Surface).Scan(&claims, &layout, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if claims != s.ClaimsEpoch || layout != s.LayoutEpoch || revision != s.Revision {
		return false, nil
	}
	if len(proposals) > 0 {
		inserts := &pgx.Batch{}
		for _, p := range proposals {
			inserts.Queue(`INSERT INTO planet_landing_sites(colony,surface,sector,geography_revision,x_q,y_q,spawn_x_q,spawn_y_q) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(colony) DO NOTHING RETURNING id`, p.Claim.ID, s.Surface, p.Claim.Sector, s.Revision, p.Placement.X, p.Placement.Y, p.Placement.SpawnX, p.Placement.SpawnY)
		}
		replies := tx.SendBatch(ctx, inserts)
		ids := make([]int64, len(proposals))
		for i, p := range proposals {
			if err = replies.QueryRow().Scan(&ids[i]); err != nil {
				replies.Close()
				return false, err
			}
			results[p.Component].Sites = append(results[p.Component].Sites, strconv.FormatInt(ids[i], 10))
		}
		if err = replies.Close(); err != nil {
			return false, err
		}
		notices := &pgx.Batch{}
		for i, p := range proposals {
			payload, _ := json.Marshal(map[string]any{"surface": s.Surface, "star": s.Star, "planet": s.Name, "planet_index": s.Planet, "sector": p.Claim.Sector, "landing_site": strconv.FormatInt(ids[i], 10)})
			notices.Queue(`INSERT INTO notifications(user_id,kind,level,message,data) SELECT u.id,'landing_ready','success','Посадочная площадка готова',$1 FROM users u LEFT JOIN companies c ON c.id=u.company_id LEFT JOIN company_roles r ON r.id=u.company_role AND r.company_id=u.company_id WHERE ($2='user' AND u.id=$3) OR ($2='company' AND u.company_id=$3 AND (c.owner_id=u.id OR COALESCE(r.permissions,'[]'::jsonb)?'planetary.enter'))`, payload, p.Claim.Owner.Kind, p.Claim.Owner.ID)
		}
		if err = tx.SendBatch(ctx, notices).Close(); err != nil {
			return false, err
		}
	}

	raw, _ := json.Marshal(results)
	bad, _ := json.Marshal(issues)
	_, err = tx.Exec(ctx, `UPDATE planet_runtime_controls SET processed_epoch=$2,layout_epoch=layout_epoch+1,policy_version=$3,components=$4,issues=$5,retry_at=now()+interval '60 seconds' WHERE surface=$1`, s.Surface, s.ClaimsEpoch, terrain.PolicyVersion, raw, bad)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// RunLandingWorker has a durable DB queue. It does not share the cosmic tick tx.
func (a *Service) RunLandingWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, done := context.WithTimeout(ctx, 20*time.Second)
			_, err := a.ReconcileLandingSites(work)
			done()
			if err != nil && ctx.Err() == nil {
				log.Printf("landing infrastructure: reconciliation failed")
				var failed *landingWorkError
				if errors.As(err, &failed) {
					retry, cancel := context.WithTimeout(ctx, 2*time.Second)
					_, _ = a.DB.Exec(retry, `UPDATE planet_runtime_controls SET retry_at=now()+interval '15 seconds' WHERE surface=$1`, failed.Surface)
					cancel()
				}
			}
		}
	}
}
