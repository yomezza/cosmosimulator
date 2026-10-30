package fleet

import (
	"context"
	"errors"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

var ErrReserved = errors.New("Корабль закреплён за планетарным визитом")

func Reserved(ctx context.Context, q postgres.Querier, ship int64) (bool, error) {
	var yes bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM planet_transfer_controls WHERE reserved_ship=$1)`, ship).Scan(&yes)
	return yes, err
}
func Available(ctx context.Context, q postgres.Querier, ship int64) error {
	yes, err := Reserved(ctx, q, ship)
	if err != nil {
		return err
	}
	if yes {
		return ErrReserved
	}
	return nil
}
