// Package ideas owns player proposals and their administrative review.
package ideas

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yomezza/cosmosimulator/internal/account"
)

// Service has explicit dependencies; the composition root owns their lifetime.
type Service struct {
	DB   *pgxpool.Pool
	Auth *account.Service
}
