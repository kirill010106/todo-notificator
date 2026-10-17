// Package clientip resolves forwarded addresses only through configured proxies.
package clientip

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type key struct{}

func peer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return host
}

// Address never falls back to unverified request headers.
func Address(r *http.Request) string {
	if addr, ok := r.Context().Value(key{}).(string); ok {
		return addr
	}
	return peer(r)
}

func New(proxies []netip.Prefix) func(http.Handler) http.Handler {
	trusted := func(addr netip.Addr) bool {
		for _, prefix := range proxies {
			if prefix.Contains(addr.Unmap()) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resolved := peer(r)
			addr, err := netip.ParseAddr(resolved)
			if err == nil && trusted(addr) {
				// Walk right-to-left: the first untrusted hop is the client.
				// Malformed chains fail closed to the immediate peer.
				chain := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
				if chain == "" {
					chain = r.Header.Get("X-Real-IP")
				}
				if chain != "" && len(chain) <= 4096 {
					parts := strings.Split(chain, ",")
					parsed := make([]netip.Addr, len(parts))
					valid := true
					for i, part := range parts {
						parsed[i], err = netip.ParseAddr(strings.TrimSpace(part))
						if err != nil || parsed[i].Zone() != "" {
							valid = false
							break
						}
					}
					if valid {
						for i := len(parsed) - 1; i >= 0 && trusted(addr); i-- {
							addr = parsed[i].Unmap()
						}
						resolved = addr.String()
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, resolved)))
		})
	}
}
