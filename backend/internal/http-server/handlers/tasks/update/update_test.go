package update

import (
	"context"
	"encoding/json"
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

type mockUpdater struct {
	err error

	called bool

	getTaskFunc         func(ctx context.Context, userID int64, taskID int64) (domain.Task, error)
	applyStatsDeltaFunc func(ctx context.Context, userID int64, delta domain.StatsDelta) error
}

func (m *mockUpdater) UpdateTask(ctx context.Context, userID int64, taskID int64, task domain.TaskUpdate) error {
	m.called = true
	return m.err
}

func (m *mockUpdater) GetTask(ctx context.Context, userID int64, taskID int64) (domain.Task, error) {
	if m.getTaskFunc != nil {
		return m.getTaskFunc(ctx, userID, taskID)
	}
	return domain.Task{Status: domain.TaskStatusPending, RewardClaimed: false}, nil
}

func (m *mockUpdater) ApplyStatsDelta(ctx context.Context, userID int64, delta domain.StatsDelta) error {
	if m.applyStatsDeltaFunc != nil {
		return m.applyStatsDeltaFunc(ctx, userID, delta)
	}
	return nil
}

func TestUpdate_Unauthorized(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), &mockUpdater{}, "", "", nil)

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"title":"a"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdate_EmptyBodyFields(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, "", "", nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{}`))
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

	updater := &mockUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, "", "", nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"title":"new"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, updater.called)
}

func TestUpdate_Success_NotifiesScheduler(t *testing.T) {
	secret := "secret"
	webhookSecret := "webhook-secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	triggered := make(chan struct{}, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)

		if r.Method == http.MethodPost &&
			r.URL.Path == "/webhook/task-created" &&
			r.Header.Get("X-Webhook-Secret") == webhookSecret &&
			payload.Type == "task_updated" {
			triggered <- struct{}{}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	updater := &mockUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, webhookSrv.URL, webhookSecret, nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"title":"new"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, updater.called)

	select {
	case <-triggered:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("expected scheduler webhook to be called after task update")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockUpdater{err: storage.ErrTaskNotFound}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, "", "", nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"title":"new"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdate_InternalError(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockUpdater{err: errors.New("boom")}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, "", "", nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"title":"new"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdate_CategoryNotFound(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockUpdater{err: storage.ErrCategoryNotFound}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, "", "", nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"category_id":999}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "category not found")
}

func TestUpdate_WithCategory(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	updater := &mockUpdater{}
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Patch("/tasks/{task_id}", New(slog.New(slog.DiscardHandler), updater, "", "", nil))

	req := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(`{"category_id":5}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, updater.called)
}
