package planetserver

import (
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	defaults := map[string]string{"DATABASE_URL": "postgres://private:do-not-log@127.0.0.1/game"}
	load := func(extra map[string]string) (Config, error) {
		return LoadConfig(func(k string) string {
			if v, ok := extra[k]; ok {
				return v
			}
			return defaults[k]
		})
	}
	c, err := load(nil)
	if err != nil || c.Addr != "127.0.0.1:8081" {
		t.Fatal("defaults", err)
	}
	c, err = load(map[string]string{"PUBLIC_ORIGIN": "https://game.example/", "PLANET_HTTP_ADDR": "0.0.0.0:8081"})
	if err != nil || c.AllowedOrigins[0] != "https://game.example" {
		t.Fatal("origin fallback", err)
	}
	for _, bad := range []map[string]string{
		{"DATABASE_URL": ""}, {"PLANET_HTTP_ADDR": "127.0.0.1:0"}, {"PLANET_HTTP_ADDR": ":70000"},
		{"PLANET_DB_TIMEOUT": "nonsense"}, {"PLANET_LEASE_TTL": "1s"}, {"PLANET_LEASE_RENEW": "20s"},
		{"PLANET_ALLOWED_ORIGINS": "*"}, {"PLANET_ALLOWED_ORIGINS": "https://a.example/private"},
		{"PLANET_ALLOWED_ORIGINS": "https://user:secret@a.example"}, {"PLANET_SHUTDOWN_TIMEOUT": "0s"},
	} {
		_, err = load(bad)
		if err == nil {
			t.Fatal("accepted invalid config", bad)
		}
		if strings.Contains(err.Error(), "do-not-log") || strings.Contains(err.Error(), "secret") {
			t.Fatal("secret in error")
		}
	}
}
