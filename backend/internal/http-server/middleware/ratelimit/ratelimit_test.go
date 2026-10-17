package ratelimit

import (
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	j "github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
