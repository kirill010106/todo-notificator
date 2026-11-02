package ratelimit

import (
	"encoding/json"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	j "github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimitResponseWithoutNormalizer(t *testing.T) {
	for _, path := range []string{"/api/v1/tasks", "/api/v1/webhooks/yookassa"} {
		t.Run(path, func(t *testing.T) {
			handler := middleware.RequestID(IP(1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
			for i := 0; i < 2; i++ {
				req := httptest.NewRequest("GET", path, nil)
				req.Header.Set("X-Request-Id", "limited-request")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				if i == 0 {
					require.Equal(t, 204, w.Code)
					continue
				}
				require.Equal(t, 429, w.Code)
				require.Equal(t, "60", w.Header().Get("Retry-After"))
				require.Equal(t, "1", w.Header().Get("X-RateLimit-Limit"))
				require.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
				require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
				if path == "/api/v1/webhooks/yookassa" {
					require.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
					require.Equal(t, "Too Many Requests\n", w.Body.String())
				} else {
					var payload map[string]any
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
					require.JSONEq(t, `{"code":"RATE_LIMITED","message":"Too Many Requests","error":"Too Many Requests","request_id":"limited-request","field_errors":{}}`, w.Body.String())
					require.Equal(t, "application/json", w.Header().Get("Content-Type"))
				}
			}
		})
	}
}

func TestAccountsBehindSameIPHaveIndependentBudgets(t *testing.T) {
	handler := InvalidAuth("test")(auth.New("test")(User(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))))
	a, err := j.NewAccessToken(domain.User{ID: 1}, "test", time.Hour)
	require.NoError(t, err)
	b, err := j.NewAccessToken(domain.User{ID: 2}, "test", time.Hour)
	require.NoError(t, err)
	call := func(token string) int {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "203.0.113.1:5"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	require.Equal(t, 204, call(a))
	require.Equal(t, 204, call(a))
	require.Equal(t, 429, call(a))
	require.Equal(t, 204, call(b))
	for i := 0; i < 20; i++ {
		require.Equal(t, 401, call("invalid"))
	}
	require.Equal(t, 429, call("different-invalid"))
	require.Equal(t, 204, call(b))
}
func TestIPv6BudgetUsesPrefix(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[2001:db8:1::1]:4"
	a := ipKey(r)
	r.RemoteAddr = "[2001:db8:1::2]:4"
	require.Equal(t, a, ipKey(r))
}
