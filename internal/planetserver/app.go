package planetserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	planetruntime "github.com/yomezza/cosmosimulator/internal/planet/runtime"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
)

const Version = "0.16.0"
const Stage = "P2-2"

// App wires only planetary components. New does not start goroutines or migrate.
type App struct {
	config      Config
	logger      *slog.Logger
	coordinator *planetruntime.Coordinator
	stopping    atomic.Bool
	started     atomic.Bool
}

func New(db *pgxpool.Pool, config Config, logger *slog.Logger) (*App, error) {
	if db == nil {
		return nil, errors.New("planetary database pool is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	c, err := planetruntime.NewCoordinator(db, config.LeaseTTL, logger)
	if err != nil {
		return nil, err
	}
	return &App{config: config, logger: logger, coordinator: c}, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /planet/api/v1/health/live", func(w http.ResponseWriter, r *http.Request) {
		if a.stopping.Load() {
			a.health(w, 503, "stopping")
			return
		}
		a.health(w, 200, "live")
	})
	mux.HandleFunc("GET /planet/api/v1/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if a.stopping.Load() {
			a.health(w, 503, "stopping")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.config.DBTimeout)
		defer cancel()
		reason := a.coordinator.Ready(ctx)
		if reason != "" {
			a.health(w, 503, reason)
			return
		}
		a.health(w, 200, "ready")
	})
	// No ticket is accepted until the actual session/handoff implementation exists.
	mux.HandleFunc("GET /planet/ws/v1", func(w http.ResponseWriter, r *http.Request) { a.health(w, 503, "planetary_gameplay_not_implemented") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); origin != "" {
			allowed := false
			for _, candidate := range a.config.AllowedOrigins {
				if candidate == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				httpkit.Reply(w, 403, map[string]string{"error": "origin_not_allowed"})
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) health(w http.ResponseWriter, status int, reason string) {
	httpkit.Reply(w, status, map[string]any{"service": "cosmosimulator-planet", "version": Version, "stage": Stage, "status": reason, "gameplay_ready": false})
}
