package server

import (
	"context"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// Same-origin gateway keeps cookies/WS on the terminal origin; upstream is a
// deployment setting, never user input. Disabled installations keep cosmic alive.
func (a *App) planetProxy() http.Handler {
	raw := os.Getenv("PLANET_INTERNAL_URL")
	if raw == "" {
		raw = "http://127.0.0.1:8081"
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || strings.Trim(u.Path, "/") != "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { httpkit.Fail(w, 503, "planetary_configuration_error") })
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) { httpkit.Fail(w, 503, "runtime_unavailable") }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Transfer.Enabled {
			httpkit.Fail(w, 503, "planetary_disabled")
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
func (a *App) runTransfers(ctx context.Context) { a.Transfer.Run(ctx) }
