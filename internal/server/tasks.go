package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

func readShip(ctx context.Context, q querier, id int64) (Ship, error) {
	var s Ship
	var raw, path, way []byte
	err := q.QueryRow(ctx, `SELECT star,destination,arrival,fuel,cargo,id,name,object_id,position_mode,departure,travel_path,local_target,local_mode,local_started,local_arrival,extractor,reserved_fuel,active,waypoints,class FROM ships WHERE id=$1`, id).Scan(&s.Star, &s.Destination, &s.Arrival, &s.Fuel, &raw, &s.ID, &s.Name, &s.ObjectID, &s.PositionMode, &s.Departure, &path, &s.LocalTarget, &s.LocalMode, &s.LocalStarted, &s.LocalArrival, &s.Extractor, &s.ReservedFuel, &s.Active, &way, &s.Class)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(raw, &s.Cargo); err != nil {
		return s, err
	}
	if err = json.Unmarshal(path, &s.Path); err != nil {
		return s, err
	}
	s.Spec = s.hull()
	err = json.Unmarshal(way, &s.Waypoints)
	return s, err
}
func settleShips(ctx context.Context, tx pgx.Tx, uid int64) error {
	rows, err := tx.Query(ctx, `UPDATE ships SET star=destination,destination=NULL,arrival=NULL,object_id='entry',position_mode='approach',departure=NULL,travel_path='[]' WHERE user_id=$1 AND arrival<=now() RETURNING id,star`, uid)
	if err != nil {
		return err
	}
	type arrived struct {
		id   int64
		star int
	}
	list := []arrived{}
	for rows.Next() {
		var v arrived
		if err = rows.Scan(&v.id, &v.star); err != nil {
			rows.Close()
			return err
		}
		list = append(list, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range list {
		if err = recordEvent(ctx, tx, uid, "arrival", map[string]any{"star": v.star, "ship": v.id}); err != nil {
			return err
		}
	}
	rows, err = tx.Query(ctx, `UPDATE ships SET object_id=local_target,position_mode=local_mode,local_target=NULL,local_mode=NULL,local_started=NULL,local_arrival=NULL WHERE user_id=$1 AND local_arrival<=now() RETURNING id,star,object_id`, uid)
	if err != nil {
		return err
	}
	list = nil
	objects := []string{}
	for rows.Next() {
		var v arrived
		var obj string
		if err = rows.Scan(&v.id, &v.star, &obj); err != nil {
			rows.Close()
			return err
		}
		list = append(list, v)
		objects = append(objects, obj)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i, v := range list {
		if err = recordEvent(ctx, tx, uid, "arrival", map[string]any{"star": v.star, "ship": v.id, "object": objects[i]}); err != nil {
			return err
		}
	}
	return nil
}
func researchBusy(ctx context.Context, q querier, sid int64) bool {
	var busy bool
	_ = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM research_jobs WHERE ship_id=$1 AND NOT paused)`, sid).Scan(&busy)
	return busy
}
func startResearch(ctx context.Context, tx pgx.Tx, uid int64, s *Ship, star Star, g *Galaxy) error {
	var known bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM surveys WHERE user_id=$1 AND star=$2)`, uid, star.ID).Scan(&known); err != nil {
		return err
	}
	if known {
		return nil
	}
	var existing int64
	err := tx.QueryRow(ctx, `SELECT ship_id FROM research_jobs WHERE user_id=$1 AND star=$2`, uid, star.ID).Scan(&existing)
	if err == nil {
		if existing != s.ID {
			return errors.New("Исследование выполняет другой корабль")
		}
		_, err = tx.Exec(ctx, `UPDATE research_jobs SET paused=FALSE,next_at=now()+make_interval(secs=>seconds) WHERE ship_id=$1`, s.ID)
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	settings, err := readGameplay(ctx, tx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET credits=credits-10 WHERE id=$1 AND credits>=10`, uid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("Недостаточно кредитов")
	}
	researchSites, err := addCustomSites(ctx, tx, star, systemSites(star))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO research_jobs(ship_id,user_id,star,total,next_at,seconds) VALUES($1,$2,$3,$4,COALESCE($5,now())+make_interval(secs=>$6),$6)`, s.ID, uid, star.ID, len(researchSites), s.Arrival, settings.ScanSeconds)
	return err
}
func researchView(ctx context.Context, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, uid int64) []map[string]any {
	rows, err := db.Query(ctx, `SELECT ship_id,star,scanned,total,next_at,paused,seconds FROM research_jobs WHERE user_id=$1 ORDER BY ship_id`, uid)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var sid int64
		var star, n, total, sec int
		var at time.Time
		var paused bool
		if rows.Scan(&sid, &star, &n, &total, &at, &paused, &sec) != nil {
			break
		}
		out = append(out, map[string]any{"ship": sid, "star": star, "scanned": n, "total": total, "next_at": at, "paused": paused, "seconds": sec})
	}
	return out
}
func extractorAccepts(e, res string) bool {
	if e == "universal" {
		return true
	}
	if e == "ice" {
		return res == "water" || res == "azoth"
	}
	if e == "gas" {
		return res == "agni" || res == "carbon"
	}
	return res != "water" && res != "azoth" && res != "agni" && res != "manna" && res != "ambrosia"
}
func extractorYield(e string, star, obj int) int {
	if e == "universal" {
		return 3
	}
	grade := 1 + (star*13+obj*7)%5
	return 4 + min(2, (concentration(star, obj)/20+grade)/4)
}
func (a *App) activities(ctx context.Context) error {
	g, err := a.loadUniverse(ctx)
	if err != nil {
		return err
	}
	rows, err := a.db.Query(ctx, `SELECT DISTINCT u.id FROM users u LEFT JOIN ships s ON s.user_id=u.id LEFT JOIN research_jobs r ON r.user_id=u.id WHERE s.arrival<=now() OR s.local_arrival<=now() OR (NOT r.paused AND r.next_at<=now()) ORDER BY u.id`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = a.advancePlayer(ctx, id, g); err != nil {
			return err
		}
	}
	return a.advanceLogistics(ctx)
}
func (a *App) advancePlayer(ctx context.Context, uid int64, g *Galaxy) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784218)`); err != nil {
		return err
	}
	g, err = a.loadUniverse(ctx)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, uid); err != nil {
		return err
	}
	if err = settleShips(ctx, tx, uid); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT r.ship_id,r.star,r.scanned,r.total,r.next_at,r.seconds FROM research_jobs r JOIN ships s ON s.id=r.ship_id WHERE r.user_id=$1 AND NOT r.paused AND s.destination IS NULL AND s.local_target IS NULL AND s.star=r.star AND r.next_at<=now() ORDER BY r.ship_id FOR UPDATE OF r`, uid)
	if err != nil {
		return err
	}
	type scan struct {
		sid                 int64
		star, n, total, sec int
		at                  time.Time
	}
	jobs := []scan{}
	for rows.Next() {
		var j scan
		if err = rows.Scan(&j.sid, &j.star, &j.n, &j.total, &j.at, &j.sec); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		steps := 1 + int(time.Since(j.at).Seconds())/j.sec
		n := min(j.total, j.n+steps)
		if n == j.total {
			tag, e := tx.Exec(ctx, `INSERT INTO surveys(user_id,star,source) VALUES($1,$2,'survey') ON CONFLICT DO NOTHING`, uid, j.star)
			if e != nil {
				return e
			}
			if tag.RowsAffected() > 0 {
				if _, err = tx.Exec(ctx, `UPDATE users SET xp=xp+10 WHERE id=$1`, uid); err != nil {
					return err
				}
				if err = recordEvent(ctx, tx, uid, "research_complete", map[string]any{"star": j.star, "ship": j.sid}); err != nil {
					return err
				}
			}
			_, err = tx.Exec(ctx, `DELETE FROM research_jobs WHERE ship_id=$1`, j.sid)
		} else {
			_, err = tx.Exec(ctx, `UPDATE research_jobs SET scanned=$2,next_at=next_at+make_interval(secs=>$3) WHERE ship_id=$1`, j.sid, n, steps*j.sec)
		}
		if err != nil {
			return err
		}
	}
	rows, err = tx.Query(ctx, `SELECT id FROM ships s WHERE user_id=$1 AND destination IS NULL AND local_target IS NULL AND jsonb_array_length(waypoints)>0 AND NOT EXISTS(SELECT 1 FROM mining_jobs WHERE ship_id=s.id) ORDER BY id`, uid)
	if err != nil {
		return err
	}
	sids := []int64{}
	for rows.Next() {
		var sid int64
		if err = rows.Scan(&sid); err != nil {
			rows.Close()
			return err
		}
		sids = append(sids, sid)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, sid := range sids {
		s, e := readShip(ctx, tx, sid)
		if e != nil {
			return e
		}
		if len(s.Waypoints) == 0 {
			continue
		}
		path := g.path(s.Star, s.Waypoints[0])
		if len(path) < 2 {
			return fmt.Errorf("invalid persisted route")
		}
		cost := s.hull().JumpFuel * (len(path) - 1)
		if s.Fuel < cost {
			return errors.New("reserved route fuel missing")
		}
		raw, _ := json.Marshal(s.Waypoints[1:])
		route, _ := json.Marshal(path)
		_, err = tx.Exec(ctx, `UPDATE ships SET fuel=fuel-$2,reserved_fuel=GREATEST(0,reserved_fuel-$2),destination=$3,departure=now(),arrival=now()+make_interval(secs=>$4),travel_path=$5,waypoints=$6 WHERE id=$1`, sid, cost, s.Waypoints[0], s.hull().JumpSeconds*(len(path)-1), route, raw)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
