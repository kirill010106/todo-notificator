package login

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
	"golang.org/x/crypto/bcrypt"
)

type mockUserProvider struct {
	user domain.User
	err  error

	saveErr error
}

func (m mockUserProvider) User(ctx context.Context, email string) (domain.User, error) {
	if m.err != nil {
		return domain.User{}, m.err
	}
	return m.user, nil
}

func (m mockUserProvider) SaveRefreshToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	return m.saveErr
}

func TestLogin_Success(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	provider := mockUserProvider{user: domain.User{ID: 1, Email: "user@test.com", PassHash: hash}}
	cfg := &config.Config{AppSecret: "app-secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour}
	h := New(slog.New(slog.DiscardHandler), provider, cfg, nil)

	body := `{"email":"user@test.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"OK"`)
	require.Contains(t, w.Body.String(), `"access_token"`)
	require.Contains(t, w.Body.String(), `"refresh_token"`)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	provider := mockUserProvider{err: storage.ErrUserNotFound}
	cfg := &config.Config{AppSecret: "app-secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour}
	h := New(slog.New(slog.DiscardHandler), provider, cfg, nil)

	body := `{"email":"user@test.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "invalid credentials")
}

func TestLogin_InternalProviderError(t *testing.T) {
	provider := mockUserProvider{err: errors.New("db down")}
	cfg := &config.Config{AppSecret: "app-secret", AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour}
	h := New(slog.New(slog.DiscardHandler), provider, cfg, nil)

	body := `{"email":"user@test.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "internal error")
}
