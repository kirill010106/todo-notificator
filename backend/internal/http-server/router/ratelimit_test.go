package router_test

import (
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRateLimitBudgetsAndTrustedProxy(t *testing.T) {
	h := router.New(router.Config{Log: slog.New(slog.DiscardHandler), Storage: &postgres.Storage{}, AppConfig: &config.Config{AppSecret: "test", HTTPServer: config.HTTPServer{TrustedProxies: []string{"172.20.0.2/32"}}}})
	request := func(peer, xff, method, path string) int {
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-For", xff)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 100; i++ {
		require.Equal(t, 200, request("172.20.0.2:5", "203.0.113.1", "GET", "/ping"))
	}
	require.Equal(t, 429, request("172.20.0.2:5", "203.0.113.1", "GET", "/ping"))
	require.Equal(t, 200, request("172.20.0.2:5", "203.0.113.2", "GET", "/ping"))
	// Public exhaustion does not consume login or payment callback budgets.
	for i := 0; i < 20; i++ {
		require.Equal(t, 400, request("172.20.0.2:5", "203.0.113.1", "POST", "/api/v1/login"))
	}
	require.Equal(t, 429, request("172.20.0.2:5", "203.0.113.1", "POST", "/api/v1/login"))
	require.NotEqual(t, 429, request("172.20.0.2:5", "203.0.113.1", "POST", "/api/v1/webhooks/yookassa"))
	// A direct client cannot obtain a fresh budget by forging forwarded headers.
	for i := 0; i < 100; i++ {
		require.Equal(t, 200, request("198.51.100.1:5", "203.0.113.3", "GET", "/ping"))
	}
	require.Equal(t, 429, request("198.51.100.1:5", "203.0.113.4", "GET", "/ping"))
}
