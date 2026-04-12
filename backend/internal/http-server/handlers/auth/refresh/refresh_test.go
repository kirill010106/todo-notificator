package refresh

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

type mockRefresher struct {
	user      *domain.User
	rotateErr error
}

func (m mockRefresher) RotateRefreshToken(ctx context.Context, oldToken string, newToken string, expiresAt time.Time) (*domain.User, error) {
	if m.rotateErr != nil {
		return nil, m.rotateErr
	}
	return m.user, nil
}

func TestRefresh_InvalidRefreshToken(t *testing.T) {
	cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour}
	h := New(slog.New(slog.DiscardHandler), mockRefresher{rotateErr: storage.ErrRefreshTokenInvalid}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refresh_token":"bad"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefresh_Success(t *testing.T) {
	cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour}
	h := New(slog.New(slog.DiscardHandler), mockRefresher{
		user: &domain.User{ID: 11, Email: "u@test.com"},
	}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refresh_token":"good"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"access_token"`)
	require.Contains(t, w.Body.String(), `"refresh_token"`)
}

func TestRefresh_RotateError(t *testing.T) {
	cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour}
	h := New(slog.New(slog.DiscardHandler), mockRefresher{
		rotateErr: errors.New("rotate failed"),
	}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refresh_token":"good"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
