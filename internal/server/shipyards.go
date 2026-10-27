package server

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func yardShipPrice(c ShipClass, star int, station string) int64 {
	if c.ID == "pilgrim" {
		return c.Price
	}
	return (c.Price*int64(100+catalogueHash(star, station)%11) + 99) / 100
}

// A catalogue expansion is a one-time delivery, never a periodic restock. Keep
// existing offers (including sold-out ones), balances and purchased ships intact.
func seedShipyards(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT star,object_id,ship_stock,yard_seeded,hull_catalogue_version FROM stations WHERE shipyard AND active ORDER BY star,object_id`)
	if err != nil {
		return err
	}
	type yard struct {
		star, stock, version int
		station              string
		seeded               bool
	}
	var yards []yard
	for rows.Next() {
		var y yard
		if err = rows.Scan(&y.star, &y.station, &y.stock, &y.seeded, &y.version); err != nil {
			rows.Close()
			return err
		}
		yards = append(yards, y)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	ordinal := 0
	for _, y := range yards {
		h := int(catalogueHash(y.star, y.station))
		models := map[string]bool{"pilgrim": true, shipClasses[1+h%4].ID: true}
		if y.star == 1 && y.station == "station" {
			models = map[string]bool{"pilgrim": true, "wanderer": true, "vesper": true}
		} else if y.star == 5 && y.station == "station" {
			models = map[string]bool{"pilgrim": true, "forge": true, "crucible": true}
		} else {
			// The default network covers all ten new hulls in five yards;
			// individual yards retain a limited selection of models.
			models = map[string]bool{"pilgrim": true, shipClasses[1+ordinal%4].ID: true}
			models[shipClasses[5+(ordinal*2)%10].ID] = true
			models[shipClasses[5+(ordinal*2+1)%10].ID] = true
			ordinal++
		}
		if y.seeded && y.version >= hullCatalogueVersion {
			continue
		}
		for _, c := range shipClasses {
			price := yardShipPrice(c, y.star, y.station)
			// Reprice every legacy offer once, even if it is not selected
			// for a newly generated yard. Do not touch its remaining stock.
			batch.Queue(`UPDATE shipyard_stock SET price=$4 WHERE star=$1 AND station=$2 AND ship_class=$3`, y.star, y.station, c.ID, price)
			if !models[c.ID] || y.seeded && isOriginalHull(c.ID) {
				continue
			}
			quantity := 1 + (h+len(c.ID))%2
			if c.ID == "pilgrim" {
				quantity = y.stock
			}
			batch.Queue(`INSERT INTO shipyard_stock(star,station,ship_class,quantity,price) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, y.star, y.station, c.ID, quantity, price)
		}
		batch.Queue(`UPDATE stations SET yard_seeded=TRUE,hull_catalogue_version=$3 WHERE star=$1 AND object_id=$2`, y.star, y.station, hullCatalogueVersion)
	}
	if batch.Len() == 0 {
		return nil
	}
	return tx.SendBatch(ctx, batch).Close()
}

func isOriginalHull(id string) bool {
	return id == "pilgrim" || id == "wanderer" || id == "forge" || id == "ark" || id == "hermes"
}
