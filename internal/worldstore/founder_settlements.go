package worldstore

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/universe"
)

func UpgradeFounderSettlements(ctx context.Context, tx pgx.Tx, g *universe.Galaxy) (bool, error) {
	if g.SettlementVersion >= 1 {
		return false, nil
	}
	for _, d := range g.Catalog {
		if d.Opened {
			universe.AssignGalaxyContent(g, d.ID)
		}
	}
	if err := ReconcileStations(ctx, tx, g); err != nil {
		return false, err
	}
	g.SettlementVersion = 1
	g.Revision++
	return true, nil
}
