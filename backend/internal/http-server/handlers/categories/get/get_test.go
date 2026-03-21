package get

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

type mockCategoryGetter struct {
	categories []domain.Category
	err        error

	called    bool
	gotUserID int64
}

func (m *mockCategoryGetter) GetCategories(ctx context.Context, userID int64) ([]domain.Category, error) {
	m.called = true
	m.gotUserID = userID
	return m.categories, m.err
}

func TestGet_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), &mockCategoryGetter{})

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGet_Success(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	getter := &mockCategoryGetter{
		categories: []domain.Category{
			{ID: 1, UserID: 5, Name: "Work"},
			{ID: 2, UserID: 5, Name: "Health"},
		},
	}

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/categories", New(slog.New(slog.DiscardHandler), getter))

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, getter.called)
	require.Equal(t, int64(5), getter.gotUserID)
	require.Contains(t, w.Body.String(), `"name":"Work"`)
	require.Contains(t, w.Body.String(), `"name":"Health"`)
}

func TestGet_NotFound(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	getter := &mockCategoryGetter{err: storage.ErrCategoryNotFound}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/categories", New(slog.New(slog.DiscardHandler), getter))

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestGet_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	getter := &mockCategoryGetter{err: errors.New("boom")}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/categories", New(slog.New(slog.DiscardHandler), getter))

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
