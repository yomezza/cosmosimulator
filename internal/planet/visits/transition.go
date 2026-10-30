package visits

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yomezza/cosmosimulator/internal/planet/access"
	"github.com/yomezza/cosmosimulator/internal/planet/scene"
	"github.com/yomezza/cosmosimulator/internal/planet/scenestore"
)

var ErrOffer = errors.New("offer_expired")
var ErrOperation = errors.New("transition_operation_conflict")

const EdgeRadius = 3 * 1024

func Portal(s *scene.Scene, neighbor int) (scene.Portal, bool) {
	for _, p := range s.Portals() {
		if p.Neighbor == neighbor {
			return p, true
		}
	}
	return scene.Portal{}, false
}
func Near(p scene.Portal, x, y int) bool {
	return math.Hypot(float64(p.Ingress.X-x), float64(p.Ingress.Y-y)) <= EdgeRadius
}
func Pair(from, to *scene.Scene) (scene.Portal, scene.Portal, error) {
	a, b := from.Key(), to.Key()
	p, ok := Portal(from, b.Sector)
	q, back := Portal(to, a.Sector)
	if !ok || !back || a.Surface != b.Surface || a.Revision != b.Revision || a.Sector == b.Sector || math.Abs(p.WorldX-q.WorldX) > 1e-8 || math.Abs(p.WorldY-q.WorldY) > 1e-8 || !from.Walkable(p.Ingress.X/1024, p.Ingress.Y/1024) || !to.Walkable(q.Ingress.X/1024, q.Ingress.Y/1024) {
		return p, q, ErrOffer
	}
	return p, q, nil
}

// Transition has no scene loads or network waits inside the transaction. The
// caller owns frozen RAM coordinates, source/destination pins and the user gate.
func (a *Service) Transition(ctx context.Context, id access.Identity, w Welcome, f scenestore.Fence, from, to *scene.Scene, x, y int, offer, operation string) (Welcome, error) {
	if !access.ValidUUID(offer) || !access.ValidUUID(operation) {
		return w, ErrOffer
	}
	p, q, err := Pair(from, to)
	if err != nil || !Near(p, x, y) || !from.Walkable(x/1024, y/1024) {
		return w, ErrOffer
	}
	tx, err := a.locked(ctx, id.User, f)
	if err != nil {
		return w, err
	}
	defer tx.Rollback(context.Background())
	if err = check(ctx, tx, id, w, f); err != nil {
		return w, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM planet_transition_receipts WHERE user_id=$1 AND operation=$2)`, id.User, operation).Scan(&exists); err != nil {
		return w, err
	}
	if exists {
		return w, ErrOperation
	}
	if err = scenestore.CheckSource(ctx, tx, from); err != nil {
		return w, err
	}
	if err = scenestore.CheckSource(ctx, tx, to); err != nil {
		return w, err
	}
	territory, err := access.Sector(ctx, tx, id.User, to.Key())
	if err != nil {
		return w, err
	}
	k, n := from.Key(), to.Key()
	err = tx.QueryRow(ctx, `UPDATE planet_visits SET current_sector=$7,colony=$8,connection_epoch=connection_epoch+1 WHERE user_id=$1 AND visit_id=$2 AND connection_epoch=$3 AND generation=$4 AND current_surface=$5 AND current_sector=$6 AND revision=$9 RETURNING connection_epoch`, id.User, w.Visit, w.Epoch, w.Generation, k.Surface, k.Sector, n.Sector, territory.Colony, k.Revision).Scan(&w.Epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrReplaced
	}
	if err != nil {
		return w, err
	}
	err = tx.QueryRow(ctx, `UPDATE planet_characters SET last_sector=$6,x_q=$7,y_q=$8,position_version=position_version+1 WHERE user_id=$1 AND last_surface=$2 AND last_sector=$3 AND last_revision=$4 AND position_version=$5 RETURNING position_version`, id.User, k.Surface, k.Sector, k.Revision, w.PositionVersion, n.Sector, q.Ingress.X, q.Ingress.Y).Scan(&w.PositionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrReplaced
	}
	if err != nil {
		return w, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO planet_transition_receipts(user_id,operation,offer,visit_id,from_sector,to_sector,surface,revision,source_x_q,source_y_q,arrival_x_q,arrival_y_q,connection_epoch) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, id.User, operation, offer, w.Visit, k.Sector, n.Sector, k.Surface, k.Revision, x, y, q.Ingress.X, q.Ingress.Y, w.Epoch)
	if err != nil {
		return w, err
	}
	if err = tx.Commit(ctx); err != nil {
		return w, err
	}
	w.Time = time.Now().UnixMilli()
	w.Sector = n.Sector
	w.X = q.Ingress.X
	w.Y = q.Ingress.Y
	return w, nil
}
