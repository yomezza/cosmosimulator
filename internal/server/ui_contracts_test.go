package server

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// No pool or session cookie is supplied: these routes must reject access
// before touching PostgreSQL. The browser fixture cannot prove server auth.
func TestUIAnonymousAPIBoundary(t *testing.T) {
	h := (&App{}).routes()
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/state"},
		{"GET", "/api/fleet"},
		{"GET", "/api/galaxy"},
		{"GET", "/api/system?id=1"},
		{"GET", "/api/workbench"},
		{"GET", "/api/admin/users?page=1&q=pilot"},
		{"GET", "/api/planet/surface?star=1&planet=planet-1"},
		{"GET", "/api/planet/state?star=1&planet=planet-1"},
		{"POST", "/api/admin/tick"},
		{"POST", "/api/planet/action"},
		{"POST", "/api/planner"},
		{"POST", "/api/logistics"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated route returned %d: %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private API response must not be cached")
			}
		})
	}
}

func TestUIEmbeddedDelivery(t *testing.T) {
	h := (&App{}).routes()
	for _, resource := range []struct{ path, media string }{
		{"/", "text/html"},
		{"/app.js", "text/javascript"},
		{"/windows.js", "text/javascript"},
		{"/controls.js", "text/javascript"},
		{"/planet-loader.js", "text/javascript"},
		{"/planet.js", "text/javascript"},
		{"/ru.json", "application/json"},
		{"/en.json", "application/json"},
		{"/terminal033.css", "text/css"},
		{"/pilgrim.svg", "image/svg+xml"},
		{"/surface-ground-grass.png", "image/png"},
	} {
		t.Run(resource.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", resource.path, nil))
			media, _, err := mime.ParseMediaType(w.Header().Get("Content-Type"))
			if w.Code != http.StatusOK || err != nil || media != resource.media || w.Body.Len() == 0 {
				t.Fatalf("embedded asset: status=%d media=%q bytes=%d err=%v", w.Code, media, w.Body.Len(), err)
			}
			if !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
				t.Fatal("static UI must preserve the same-origin script policy")
			}
		})
	}
}
