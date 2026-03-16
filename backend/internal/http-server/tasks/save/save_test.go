package save

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

type mockTaskSaver struct {
	id  int64
	err error

	called bool
	task   domain.Task
}

func (m *mockTaskSaver) SaveTask(ctx context.Context, t domain.Task) (int64, error) {
	m.called = true
	m.task = t
	if m.err != nil {
		return 0, m.err
	}
	return m.id, nil
}

func TestSave_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), &mockTaskSaver{}, "", "")

	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"task"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSave_Created(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockTaskSaver{id: 99}
	h := New(slog.New(slog.DiscardHandler), saver, "", "")

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/tasks", h)

	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"task"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.True(t, saver.called)
	require.Equal(t, int64(5), saver.task.UserID)
	require.Contains(t, w.Body.String(), `"id":99`)
}

func TestSave_Conflict(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockTaskSaver{err: storage.ErrTaskExists}
	h := New(slog.New(slog.DiscardHandler), saver, "", "")

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/tasks", h)

	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"task"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestSave_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockTaskSaver{err: errors.New("db down")}
	h := New(slog.New(slog.DiscardHandler), saver, "", "")

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/tasks", h)

	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"task"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}
