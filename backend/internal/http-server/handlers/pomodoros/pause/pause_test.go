package pause

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

type mockPomodoroProvider struct {
	addBreakErr error

	addBreakCalled  bool
	calledUserID    int64
	calledSessionID int64
}

func (m *mockPomodoroProvider) AddPomodoroBreak(ctx context.Context, userID int64, sessionID int64) error {
	m.addBreakCalled = true
	m.calledUserID = userID
	m.calledSessionID = sessionID
	return m.addBreakErr
}

func (m *mockPomodoroProvider) GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error) {
	return nil, nil // Dummy depending on provider definition
}

func setupTestRouter(h http.HandlerFunc, secret string) *chi.Mux {
	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/pomodoros/{id}/pause", h)
	return router
}

func getTestToken(id int64, email, secret string) string {
	tok, _ := jwt.NewAccessToken(domain.User{ID: id, Email: email}, secret, time.Hour)
	return tok
}

func TestPause_Success(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/pause", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, provider.addBreakCalled)
	require.Equal(t, int64(5), provider.calledUserID)
	require.Equal(t, int64(10), provider.calledSessionID)
}

func TestPause_Exhausted(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{
		addBreakErr: storage.ErrBreakExhausted,
	}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/pause", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestPause_NotFound(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{
		addBreakErr: storage.ErrSessionNotFound,
	}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/pause", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPause_InternalError(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{
		addBreakErr: errors.New("db error"),
	}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/pause", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
