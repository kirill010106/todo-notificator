package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware_UnauthorizedWithoutHeader(t *testing.T) {
	mw := New("secret")

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_SetsUserIDWithValidToken(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 123, Email: "u@t.com"}, secret, time.Hour)
	require.NoError(t, err)

	mw := New(secret)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := GetUserID(r.Context())
		require.True(t, ok)
		require.Equal(t, int64(123), uid)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}
