// Package terminal owns the cosmic read model and HTTP command dispatch.
package terminal

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yomezza/cosmosimulator/internal/account"
	"github.com/yomezza/cosmosimulator/internal/commerce"
	"github.com/yomezza/cosmosimulator/internal/corporations"
	"github.com/yomezza/cosmosimulator/internal/navigation"
	"github.com/yomezza/cosmosimulator/internal/worldstore"
)

// Service has explicit dependencies; the composition root owns their lifetime.
type Service struct {
	DB           *pgxpool.Pool
	Auth         *account.Service
	Commerce     *commerce.Service
	Corporations *corporations.Service
	Navigation   *navigation.Service
	World        *worldstore.Service
}
