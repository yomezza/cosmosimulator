// Package benchmark owns isolated HTTP workloads in disposable schemas.
package benchmark

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yomezza/cosmosimulator/internal/account"
)

// Service has explicit dependencies; the composition root owns their lifetime.
type Service struct {
	DB           *pgxpool.Pool
	Auth         *account.Service
	OpenDatabase func(context.Context, string, string) (*pgxpool.Pool, error)
	Handler      func(context.Context, *pgxpool.Pool) (http.Handler, error)
}
