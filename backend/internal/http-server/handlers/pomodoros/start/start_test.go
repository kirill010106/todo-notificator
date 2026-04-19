package start

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	startErr          error
	session           *domain.PomodoroSession
	
	getActiveErr      error
	activeSession     *domain.PomodoroSession

	startCalled       bool
	getActiveCalled   bool
	startedUserID     int64
	startedTaskID     *int64
}

func (m *mockPomodoroProvider) StartPomodoroSession(ctx context.Context, userID int64, taskID *int64) (*domain.PomodoroSession, error) {
	m.startCalled = true
	m.startedUserID = userID
	m.startedTaskID = taskID
	return m.session, m.startErr
}

func (m *mockPomodoroProvider) GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error) {
	m.getActiveCalled = true
	return m.activeSession, m.getActiveErr
}

func setupTestRouter(h http.HandlerFunc, secret string) *chi.Mux {
	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/pomodoros/start", h)
	return router
}

func getTestToken(id int64, email, secret string) string {
	tok, _ := jwt.NewAccessToken(domain.User{ID: id, Email: email}, secret, time.Hour)
	return tok
}

func TestStart_Created(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	now := time.Now()
	taskID := int64(10)
	session := &domain.PomodoroSession{
		ID:              1,
		UserID:          5,
		TaskID:          &taskID,
		Status:          domain.PomodoroStatusActive,
		StartedAt:       &now,
		DurationMinutes: 25,
	}

	provider := &mockPomodoroProvider{session: session}
	h := New(slog.New(slog.DiscardHandler), provider, nil)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/start", strings.NewReader(`{"task_id":10}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.True(t, provider.startCalled)
	require.Equal(t, int64(5), provider.startedUserID)
	require.NotNil(t, provider.startedTaskID)
	require.Equal(t, int64(10), *provider.startedTaskID)
}

func TestStart_ConflictActiveSession(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	active := &domain.PomodoroSession{ID: 99, UserID: 5, Status: domain.PomodoroStatusActive}
	provider := &mockPomodoroProvider{
		startErr:      storage.ErrSessionActive,
		activeSession: active,
	}
	h := New(slog.New(slog.DiscardHandler), provider, nil)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/start", strings.NewReader(`{"task_id":10}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	require.True(t, provider.startCalled)
	require.True(t, provider.getActiveCalled)
	require.Contains(t, w.Body.String(), "99") // ID of active session should be in response
}

func TestStart_InternalError(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{
		startErr: errors.New("db error"),
	}
	h := New(slog.New(slog.DiscardHandler), provider, nil)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/start", strings.NewReader(`{"task_id":10}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
