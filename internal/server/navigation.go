package server

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

func docked(s Ship) bool {
	return s.Destination == nil && s.LocalTarget == nil && (strings.HasPrefix(s.ObjectID, "station") || strings.HasPrefix(s.ObjectID, "project-")) && s.PositionMode == "docked"
}
func nearObject(s Ship, id string) bool {
	return s.Destination == nil && s.LocalTarget == nil && s.ObjectID == id
}
func (a *App) fleet(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var id int64
	if err = tx.QueryRow(r.Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&id); err != nil {
		a.internal(w, err)
		return
	}
	s, err := ensureShip(r.Context(), tx, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	_ = s
	rows, err := a.db.Query(r.Context(), `SELECT id FROM ships WHERE user_id=$1 ORDER BY id`, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			a.internal(w, err)
			return
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		a.internal(w, err)
		return
	}
	ships := []Ship{}
	for _, id := range ids {
		ship, e := readShip(r.Context(), a.db, id)
		if e != nil {
			a.internal(w, e)
			return
		}
		ships = append(ships, ship)
	}
	reply(w, 200, map[string]any{"ships": ships, "server_time": time.Now()})
}
func saveShip(ctx context.Context, tx pgx.Tx, id int64, s Ship) error {
	cargo, _ := json.Marshal(s.Cargo)
	path, _ := json.Marshal(s.Path)
	_, err := tx.Exec(ctx, `UPDATE ships SET destination=$2,arrival=$3,fuel=$4,cargo=$5,object_id=$6,position_mode=$7,departure=$8,travel_path=$9,local_target=$10,local_mode=$11,local_started=$12,local_arrival=$13 WHERE id=$1 AND user_id=$14`, s.ID, s.Destination, s.Arrival, s.Fuel, cargo, s.ObjectID, s.PositionMode, s.Departure, path, s.LocalTarget, s.LocalMode, s.LocalStarted, s.LocalArrival, id)
	return err
}
