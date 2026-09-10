package active

import (
	"context"
	"encoding/json"
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

type mockPomodoroGetter struct {
	session *domain.PomodoroSession
	err     error
	called  bool
	userID  int64
}

func (m *mockPomodoroGetter) GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error) {
	m.called = true
	m.userID = userID
	return m.session, m.err
}

func TestNew_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	now := time.Now().UTC()
	taskID := int64(10)
	getter := &mockPomodoroGetter{
		session: &domain.PomodoroSession{
			ID:              1,
			UserID:          42,
			TaskID:          &taskID,
			Status:          domain.PomodoroStatusActive,
			StartedAt:       &now,
			DurationMinutes: 25,
		},
	}

	h := New(logger, getter)

	secret := "test-secret"
	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "user@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Get("/pomodoros/active", h)

	req := httptest.NewRequest(http.MethodGet, "/pomodoros/active", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, getter.called)
	require.Equal(t, int64(42), getter.userID)

	var resp Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "OK", resp.Status)
	require.NotNil(t, resp.ActiveSession)
	require.Equal(t, int64(1), resp.ActiveSession.ID)
}

func TestNew_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	getter := &mockPomodoroGetter{
		err: storage.ErrSessionNotFound,
	}

	h := New(logger, getter)

	secret := "test-secret"
	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "user@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Get("/pomodoros/active", h)

	req := httptest.NewRequest(http.MethodGet, "/pomodoros/active", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.True(t, getter.called)

	var resp map[string]string
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "Error", resp["status"])
	require.Equal(t, "session not found", resp["error"])
}

func TestNew_InternalError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	getter := &mockPomodoroGetter{
		err: errors.New("db connection failed"),
	}

	h := New(logger, getter)

	secret := "test-secret"
	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "user@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Get("/pomodoros/active", h)

	req := httptest.NewRequest(http.MethodGet, "/pomodoros/active", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.True(t, getter.called)
}
