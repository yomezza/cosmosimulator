// Package planetserver composes the independent planetary service.
package planetserver

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL     string
	Schema          string
	Addr            string
	AllowedOrigins  []string
	DBTimeout       time.Duration
	LeaseTTL        time.Duration
	LeaseRenew      time.Duration
	ShutdownTimeout time.Duration
	LogFile         string
}

func DefaultConfig() Config {
	return Config{Addr: "127.0.0.1:8081", DBTimeout: 2 * time.Second, LeaseTTL: 15 * time.Second, LeaseRenew: 5 * time.Second, ShutdownTimeout: 10 * time.Second, AllowedOrigins: []string{"http://127.0.0.1:8080", "http://localhost:8080"}}
}

func LoadConfig(getenv func(string) string) (Config, error) {
	c := DefaultConfig()
	c.DatabaseURL = getenv("DATABASE_URL")
	c.Schema = getenv("DB_SCHEMA")
	c.LogFile = getenv("PLANET_LOG_FILE")
	if c.DatabaseURL == "" {
		return c, errors.New("set DATABASE_URL for the planetary service")
	}
	if v := getenv("PLANET_HTTP_ADDR"); v != "" {
		c.Addr = v
	}
	for _, p := range []struct {
		name   string
		target *time.Duration
	}{
		{"PLANET_DB_TIMEOUT", &c.DBTimeout}, {"PLANET_LEASE_TTL", &c.LeaseTTL}, {"PLANET_LEASE_RENEW", &c.LeaseRenew}, {"PLANET_SHUTDOWN_TIMEOUT", &c.ShutdownTimeout},
	} {
		if v := getenv(p.name); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return c, fmt.Errorf("invalid %s duration", p.name)
			}
			*p.target = d
		}
	}
	origins := getenv("PLANET_ALLOWED_ORIGINS")
	if origins == "" {
		origins = getenv("PUBLIC_ORIGIN")
	}
	if origins != "" {
		c.AllowedOrigins = strings.Split(origins, ",")
	}
	for i, v := range c.AllowedOrigins {
		c.AllowedOrigins[i] = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	_, port, err := net.SplitHostPort(c.Addr)
	n, e := strconv.Atoi(port)
	if err != nil || e != nil || n < 1 || n > 65535 {
		return errors.New("PLANET_HTTP_ADDR must be host:port (1-65535)")
	}
	if c.DBTimeout < 100*time.Millisecond || c.DBTimeout > 30*time.Second {
		return errors.New("PLANET_DB_TIMEOUT must be 100ms-30s")
	}
	if c.LeaseTTL < 3*time.Second || c.LeaseTTL > 5*time.Minute || c.LeaseRenew < time.Second || c.LeaseRenew > c.LeaseTTL/3 || c.DBTimeout >= c.LeaseRenew {
		return errors.New("lease requires TTL 3s-5m, renewal >=1s and <=TTL/3, DB timeout < renewal")
	}
	if c.ShutdownTimeout < time.Second || c.ShutdownTimeout > time.Minute {
		return errors.New("PLANET_SHUTDOWN_TIMEOUT must be 1s-1m")
	}
	if len(c.AllowedOrigins) == 0 {
		return errors.New("at least one planetary Origin is required")
	}
	for _, s := range c.AllowedOrigins {
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.Host, "*") {
			return errors.New("planetary Origins must be explicit http(s) origins without paths or wildcards")
		}
	}
	return nil
}
