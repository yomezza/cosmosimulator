package fleet

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

func Docked(s Ship) bool {
	return s.Destination == nil && s.LocalTarget == nil && (strings.HasPrefix(s.ObjectID, "station") || strings.HasPrefix(s.ObjectID, "project-")) && s.PositionMode == "docked"
}

func NearObject(s Ship, id string) bool {
	return s.Destination == nil && s.LocalTarget == nil && s.ObjectID == id
}

func SaveShip(ctx context.Context, tx pgx.Tx, id int64, s Ship) error {
	cargo, _ := json.Marshal(s.Cargo)
	path, _ := json.Marshal(s.Path)
	_, err := tx.Exec(ctx, `UPDATE ships SET destination=$2,arrival=$3,fuel=$4,cargo=$5,object_id=$6,position_mode=$7,departure=$8,travel_path=$9,local_target=$10,local_mode=$11,local_started=$12,local_arrival=$13 WHERE id=$1 AND user_id=$14`, s.ID, s.Destination, s.Arrival, s.Fuel, cargo, s.ObjectID, s.PositionMode, s.Departure, path, s.LocalTarget, s.LocalMode, s.LocalStarted, s.LocalArrival, id)
	return err
}
