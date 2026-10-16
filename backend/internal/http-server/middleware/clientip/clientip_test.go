package clientip

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestTrustedProxyResolution(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("172.20.0.2/32"), netip.MustParsePrefix("10.1.0.0/24"), netip.MustParsePrefix("2001:db8:1::/64")}
	cases := []struct{ name, peer, xff, real, want string }{
		{"direct spoof", "203.0.113.1:5", "185.71.76.1", "185.71.76.2", "203.0.113.1"},
		{"private is not implicitly trusted", "172.20.0.3:5", "185.71.76.1", "", "172.20.0.3"},
		{"proxy", "172.20.0.2:5", "203.0.113.2", "", "203.0.113.2"},
		{"spoofed leftmost", "172.20.0.2:5", "185.71.76.1, 203.0.113.2", "", "203.0.113.2"},
		{"trusted chain", "172.20.0.2:5", "203.0.113.2, 10.1.0.2", "", "203.0.113.2"},
		{"malformed fails closed", "172.20.0.2:5", "garbage, 203.0.113.2", "185.71.76.1", "172.20.0.2"},
		{"empty hop", "172.20.0.2:5", "203.0.113.2,", "", "172.20.0.2"},
		{"real fallback", "172.20.0.2:5", "", "203.0.113.2", "203.0.113.2"},
		{"no headers", "172.20.0.2:5", "", "", "172.20.0.2"},
		{"IPv6", "[2001:db8:1::2]:5", "2001:db8:2::5", "", "2001:db8:2::5"},
		{"mapped IPv4", "[::ffff:172.20.0.2]:5", "::ffff:203.0.113.2", "", "203.0.113.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.peer
			r.Header.Set("X-Forwarded-For", c.xff)
			r.Header.Set("X-Real-IP", c.real)
			original := r.RemoteAddr
			New(proxies)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, c.want, Address(r))
				require.Equal(t, original, r.RemoteAddr)
			})).ServeHTTP(httptest.NewRecorder(), r)
		})
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.20.0.2:5"
	r.Header.Set("X-Real-IP", "185.71.76.1")
	require.Equal(t, "172.20.0.2", Address(r))
}
