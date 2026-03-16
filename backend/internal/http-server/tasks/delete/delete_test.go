package delete

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

type mockDeleter struct {
	err error
}

func (m mockDeleter) DeleteTask(ctx context.Context, userID int64, taskID int64) error {
	return m.err
}

func TestDelete_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockDeleter{})

	req := httptest.NewRequest(http.MethodDelete, "/tasks/1", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDelete_Success(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), mockDeleter{}))

	req := httptest.NewRequest(http.MethodDelete, "/tasks/10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestDelete_NotFound(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), mockDeleter{err: storage.ErrTaskNotFound}))

	req := httptest.NewRequest(http.MethodDelete, "/tasks/10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDelete_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), mockDeleter{err: errors.New("boom")}))

	req := httptest.NewRequest(http.MethodDelete, "/tasks/10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
