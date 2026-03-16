package logout

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

type mockRevoker struct {
	deleteErr error
	allErr    error
}

func (m mockRevoker) DeleteRefreshToken(ctx context.Context, token string) error {
	return m.deleteErr
}

func (m mockRevoker) DeleteUserRefreshTokens(ctx context.Context, userID int64) error {
	return m.allErr
}

func TestLogout_SingleDeviceSuccess(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockRevoker{})

	body := `{"refresh_token":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestLogout_InvalidBody(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockRevoker{})

	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader("{"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogout_InternalError(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockRevoker{deleteErr: errors.New("db")})

	body := `{"refresh_token":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLogout_InvalidTokenStillNoContent(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockRevoker{deleteErr: storage.ErrRefreshTokenInvalid})

	body := `{"refresh_token":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}
