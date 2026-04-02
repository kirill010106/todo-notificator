package stop

import (
	"context"
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
	stopErr       error
	getActiveErr  error
	updateTaskErr error

	activeSession *domain.PomodoroSession

	stopCalled    bool
	stoppedID     int64
	stoppedStatus string

	getActiveCalled bool

	updateTaskCalled bool
	updatedUserID    int64
	updatedTaskID    int64
	updatedUpdate    domain.TaskUpdate
}

func (m *mockPomodoroProvider) StopPomodoroSession(ctx context.Context, sessionID int64, finalStatus string) error {
	m.stopCalled = true
	m.stoppedID = sessionID
	m.stoppedStatus = finalStatus
	return m.stopErr
}

func (m *mockPomodoroProvider) GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error) {
	m.getActiveCalled = true
	return m.activeSession, m.getActiveErr
}

func (m *mockPomodoroProvider) UpdateTask(ctx context.Context, userID int64, taskID int64, update domain.TaskUpdate) error {
	m.updateTaskCalled = true
	m.updatedUserID = userID
	m.updatedTaskID = taskID
	m.updatedUpdate = update
	return m.updateTaskErr
}

func setupTestRouter(h http.HandlerFunc, secret string) *chi.Mux {
	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/pomodoros/{id}/stop", h)
	return router
}

func getTestToken(id int64, email, secret string) string {
	tok, _ := jwt.NewAccessToken(domain.User{ID: id, Email: email}, secret, time.Hour)
	return tok
}

func TestStop_SuccessWithoutTaskFinish(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	taskID := int64(42)
	active := &domain.PomodoroSession{ID: 10, UserID: 5, TaskID: &taskID}

	provider := &mockPomodoroProvider{activeSession: active}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/stop", strings.NewReader(`{"action":"abandoned"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	require.True(t, provider.getActiveCalled)
	require.True(t, provider.stopCalled)
	require.Equal(t, int64(10), provider.stoppedID)
	require.Equal(t, domain.PomodoroStatusAbandoned, provider.stoppedStatus)

	require.False(t, provider.updateTaskCalled) // false because finish_task was false
}

func TestStop_SuccessWithTaskFinishCompleted(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	taskID := int64(42)
	active := &domain.PomodoroSession{ID: 10, UserID: 5, TaskID: &taskID}

	provider := &mockPomodoroProvider{activeSession: active}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/stop", strings.NewReader(`{"action":"completed", "finish_task": true}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	require.True(t, provider.updateTaskCalled)
	require.Equal(t, int64(5), provider.updatedUserID)
	require.Equal(t, taskID, provider.updatedTaskID)
	require.NotNil(t, provider.updatedUpdate.Status)
	require.Equal(t, domain.TaskStatusDone, *provider.updatedUpdate.Status)
}

func TestStop_SuccessWithTaskFinishAbandoned_SetsBurnt(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	taskID := int64(42)
	active := &domain.PomodoroSession{ID: 10, UserID: 5, TaskID: &taskID}

	provider := &mockPomodoroProvider{activeSession: active}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/stop", strings.NewReader(`{"action":"abandoned", "finish_task": true}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	require.True(t, provider.updateTaskCalled)
	require.Equal(t, int64(5), provider.updatedUserID)
	require.Equal(t, taskID, provider.updatedTaskID)
	require.NotNil(t, provider.updatedUpdate.Status)
	require.Equal(t, domain.TaskStatusBurnt, *provider.updatedUpdate.Status)
}

func TestStop_NotFoundSession(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{
		getActiveErr: storage.ErrSessionNotFound,
	}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/stop", strings.NewReader(`{"action":"completed"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, provider.stopCalled)
}

func TestStop_InvalidAction(t *testing.T) {
	secret := "secret"
	tok := getTestToken(5, "u@test.com", secret)

	provider := &mockPomodoroProvider{}
	h := New(slog.New(slog.DiscardHandler), provider)

	router := setupTestRouter(h, secret)

	req := httptest.NewRequest(http.MethodPost, "/pomodoros/10/stop", strings.NewReader(`{"action":"invalid"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, provider.getActiveCalled)
	require.False(t, provider.stopCalled)
}
