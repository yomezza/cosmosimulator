// Package commerce owns trade, reservations, deliveries and bot transactions.
package commerce

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yomezza/cosmosimulator/internal/account"
	"github.com/yomezza/cosmosimulator/internal/worldstore"
)

// Service has explicit dependencies; the composition root owns their lifetime.
type Service struct {
	DB    *pgxpool.Pool
	Auth  *account.Service
	World *worldstore.Service
}
