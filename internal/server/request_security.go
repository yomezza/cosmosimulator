package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	out := []netip.Prefix{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES requires comma-separated IP CIDRs: %w", err)
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}
func (a *App) trustedProxy(ip netip.Addr) bool {
	for _, prefix := range a.trustedProxies {
		if prefix.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (a *App) clientAddress(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	ip, err := netip.ParseAddr(peer)
	if err != nil {
		return peer
	}
	ip = ip.Unmap()
	if !a.trustedProxy(ip) {
		return ip.String()
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" || len(forwarded) > 1024 {
		return ip.String()
	}
	chain := strings.Split(forwarded, ",")
	if len(chain) > 32 {
		return ip.String()
	}
	for i := len(chain) - 1; i >= 0 && a.trustedProxy(ip); i-- {
		next, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return peer
		}
		ip = next.Unmap()
	}
	return ip.String()
}

func jsonBodyLimit(r *http.Request) int64 {
	if r.Method == "POST" && r.URL.Path == "/api/ideas" {
		return 16 * 1024
	}
	return 4096
}

// Read the entire bounded JSON document before invoking any handler. In
// particular, a slow sender must not hold user rows or the world mutation lock.
func bufferJSON(w http.ResponseWriter, r *http.Request) bool {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, jsonBodyLimit(r)))
	if err != nil {
		fail(w, 400, "Некорректный JSON")
		return false
	}
	if !json.Valid(data) {
		fail(w, 400, "Некорректный JSON")
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return true
}

// Context deadlines bound waits for pool connections and locks even after the
// request body has already arrived. HTTP write deadlines alone do not do that.
func apiDeadline(r *http.Request) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	return r.WithContext(ctx), cancel
}
