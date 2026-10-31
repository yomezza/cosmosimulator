// Package simulation owns the single cosmic clock and activity orchestration.
package simulation

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yomezza/cosmosimulator/internal/platform/telemetry"
	"sync/atomic"

	"github.com/yomezza/cosmosimulator/internal/commerce"
	"github.com/yomezza/cosmosimulator/internal/navigation"
	"github.com/yomezza/cosmosimulator/internal/planner"
	"github.com/yomezza/cosmosimulator/internal/worldstore"
)

// Service has explicit dependencies; the composition root owns their lifetime.
type Service struct {
	TickDurations telemetry.Samples
	TickLoopLag   telemetry.Samples
	TickErrors    atomic.Uint64
	DB            *pgxpool.Pool
	Commerce      *commerce.Service
	Planner       *planner.Service
	Navigation    *navigation.Service
	World         *worldstore.Service
}
