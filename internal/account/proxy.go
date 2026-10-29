package account

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func (a *Service) trustedProxy(ip netip.Addr) bool {
	for _, prefix := range a.TrustedProxies {
		if prefix.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

func (a *Service) ClientAddress(r *http.Request) string {
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
