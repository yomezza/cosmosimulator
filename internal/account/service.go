// Package account owns accounts, session validation and authentication policy.
package account

import (
	"net/netip"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service has explicit dependencies; the composition root owns their lifetime.
type Service struct {
	DB             *pgxpool.Pool
	Secure         bool
	TrustedProxies []netip.Prefix
	mu             sync.Mutex
	attempts       map[string]attempt
}
