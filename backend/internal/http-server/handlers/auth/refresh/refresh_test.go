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
	rt      *domain.RefreshToken
	rtErr   error
	user    *domain.User
	userErr error
	saveErr error
}

func (m mockRefresher) GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	if m.rtErr != nil {
		return nil, m.rtErr
	}
	return m.rt, nil
}

func (m mockRefresher) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	if m.userErr != nil {
		return nil, m.userErr
	}
	return m.user, nil
}

func (m mockRefresher) SaveRefreshToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	return m.saveErr
}

func (m mockRefresher) DeleteRefreshToken(ctx context.Context, token string) error {
	return nil
}

func TestRefresh_InvalidRefreshToken(t *testing.T) {
	cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour}
	h := New(slog.New(slog.DiscardHandler), mockRefresher{rtErr: storage.ErrRefreshTokenInvalid}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refresh_token":"bad"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefresh_Success(t *testing.T) {
	cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour}
	h := New(slog.New(slog.DiscardHandler), mockRefresher{
		rt:   &domain.RefreshToken{UserID: 11},
		user: &domain.User{ID: 11, Email: "u@test.com"},
	}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refresh_token":"good"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"access_token"`)
	require.Contains(t, w.Body.String(), `"refresh_token"`)
}

func TestRefresh_SaveError(t *testing.T) {
	cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: time.Hour}
	h := New(slog.New(slog.DiscardHandler), mockRefresher{
		rt:      &domain.RefreshToken{UserID: 11},
		user:    &domain.User{ID: 11, Email: "u@test.com"},
		saveErr: errors.New("save failed"),
	}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refresh_token":"good"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
