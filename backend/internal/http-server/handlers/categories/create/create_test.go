package create

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

type mockCategoryCreator struct {
	id       int64
	err      error
	category domain.Category
}

func (m *mockCategoryCreator) CreateCategory(ctx context.Context, c domain.Category) (int64, error) {
	m.category = c
	if m.err != nil {
		return 0, m.err
	}
	return m.id, nil
}

func TestSave_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), &mockCategoryCreator{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"Category"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSave_Created(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockCategoryCreator{id: 99}
	h := New(slog.New(slog.DiscardHandler), saver, nil)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/categories", h)

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"Category"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	t.Log(w.Body)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, int64(5), saver.category.UserID)
	require.Contains(t, w.Body.String(), `"id":99`)
}

func TestSave_Conflict(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockCategoryCreator{err: storage.ErrCategoryExists}
	h := New(slog.New(slog.DiscardHandler), saver, nil)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/categories", h)

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"Category"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestSave_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockCategoryCreator{err: errors.New("db down")}
	h := New(slog.New(slog.DiscardHandler), saver, nil)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/categories", h)

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(`{"name":"Category"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSave_Empty(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	saver := &mockCategoryCreator{}
	h := New(slog.New(slog.DiscardHandler), saver, nil)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Post("/categories", h)

	req := httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(``))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}
