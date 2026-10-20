package auth

import (
	"context"
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

		premium, okPrem := GetPremiumStatus(r.Context())
		require.True(t, okPrem)
		require.False(t, premium)

		verified, okVer := GetVerificationStatus(r.Context())
		require.True(t, okVer)
		require.False(t, verified)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_SetsPremiumAndVerifiedTrue(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{
		ID:         777,
		Email:      "vip@t.com",
		IsVerified: true,
		IsPremium:  true,
	}, secret, time.Hour)
	require.NoError(t, err)

	mw := New(secret)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := GetUserID(r.Context())
		require.True(t, ok)
		require.Equal(t, int64(777), uid)

		premium, okPrem := GetPremiumStatus(r.Context())
		require.True(t, okPrem)
		require.True(t, premium)

		verified, okVer := GetVerificationStatus(r.Context())
		require.True(t, okVer)
		require.True(t, verified)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

type accountProvider struct{ version int64 }

func (p accountProvider) GetUserByID(context.Context, int64) (*domain.User, error) {
	return &domain.User{ID: 1, AuthVersion: p.version}, nil
}
func TestVersionRevokesAccessToken(t *testing.T) {
	token, err := jwt.NewAccessToken(domain.User{ID: 1, AuthVersion: 0}, "secret", time.Hour)
	require.NoError(t, err)
	for _, version := range []int64{0, 1} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		New("secret", accountProvider{version})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, version, GetAuthVersion(r.Context()))
			w.WriteHeader(200)
		})).ServeHTTP(w, req)
		if version == 0 {
			require.Equal(t, 200, w.Code)
		} else {
			require.Equal(t, 401, w.Code)
		}
	}
}
