package update

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

type mockCategoryUpdater struct {
	err error

	called        bool
	gotUserID     int64
	gotCategoryID int64
	gotName       string
}

func (m *mockCategoryUpdater) UpdateCategory(ctx context.Context, userID int64, categoryID int64, c domain.CategoryUpdate) error {
	m.called = true
	m.gotUserID = userID
	m.gotCategoryID = categoryID
	if c.Name != nil {
		m.gotName = *c.Name
	}
	return m.err
}

func TestUpdate_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), &mockCategoryUpdater{}, nil)

	req := httptest.NewRequest(http.MethodPatch, "/categories/1", strings.NewReader(`{"name":"Urgent"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdate_InvalidCategoryID(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockCategoryUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/categories/{category_id}", New(slog.New(slog.DiscardHandler), updater, nil))

	req := httptest.NewRequest(http.MethodPatch, "/categories/abc", strings.NewReader(`{"name":"Urgent"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, updater.called)
}

func TestUpdate_EmptyBody(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockCategoryUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/categories/{category_id}", New(slog.New(slog.DiscardHandler), updater, nil))

	req := httptest.NewRequest(http.MethodPatch, "/categories/1", strings.NewReader(``))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, updater.called)
}

func TestUpdate_NoFields(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockCategoryUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/categories/{category_id}", New(slog.New(slog.DiscardHandler), updater, nil))

	req := httptest.NewRequest(http.MethodPatch, "/categories/1", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, updater.called)
}

func TestUpdate_Success(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockCategoryUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/categories/{category_id}", New(slog.New(slog.DiscardHandler), updater, nil))

	req := httptest.NewRequest(http.MethodPatch, "/categories/10", strings.NewReader(`{"name":"Urgent"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, updater.called)
	require.Equal(t, int64(1), updater.gotUserID)
	require.Equal(t, int64(10), updater.gotCategoryID)
	require.Equal(t, "Urgent", updater.gotName)
}

func TestUpdate_NotFound(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockCategoryUpdater{err: storage.ErrCategoryNotFound}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/categories/{category_id}", New(slog.New(slog.DiscardHandler), updater, nil))

	req := httptest.NewRequest(http.MethodPatch, "/categories/1", strings.NewReader(`{"name":"Urgent"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdate_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockCategoryUpdater{err: errors.New("boom")}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/categories/{category_id}", New(slog.New(slog.DiscardHandler), updater, nil))

	req := httptest.NewRequest(http.MethodPatch, "/categories/1", strings.NewReader(`{"name":"Urgent"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
