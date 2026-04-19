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

type mockCategoryDeleter struct {
	err error

	called        bool
	gotUserID     int64
	gotCategoryID int64
}

func (m *mockCategoryDeleter) DeleteCategory(ctx context.Context, userID int64, categoryID int64) error {
	m.called = true
	m.gotUserID = userID
	m.gotCategoryID = categoryID
	return m.err
}

func TestDelete_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), &mockCategoryDeleter{}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/categories/1", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDelete_InvalidCategoryID(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	deleter := &mockCategoryDeleter{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/categories/{category_id}", New(slog.New(slog.DiscardHandler), deleter, nil))

	req := httptest.NewRequest(http.MethodDelete, "/categories/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, deleter.called)
}

func TestDelete_Success(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	deleter := &mockCategoryDeleter{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/categories/{category_id}", New(slog.New(slog.DiscardHandler), deleter, nil))

	req := httptest.NewRequest(http.MethodDelete, "/categories/10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.True(t, deleter.called)
	require.Equal(t, int64(1), deleter.gotUserID)
	require.Equal(t, int64(10), deleter.gotCategoryID)
}

func TestDelete_NotFound(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	deleter := &mockCategoryDeleter{err: storage.ErrCategoryNotFound}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/categories/{category_id}", New(slog.New(slog.DiscardHandler), deleter, nil))

	req := httptest.NewRequest(http.MethodDelete, "/categories/10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestDelete_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	deleter := &mockCategoryDeleter{err: errors.New("boom")}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Delete("/categories/{category_id}", New(slog.New(slog.DiscardHandler), deleter, nil))

	req := httptest.NewRequest(http.MethodDelete, "/categories/10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
