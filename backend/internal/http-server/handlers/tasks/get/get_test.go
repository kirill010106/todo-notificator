package get

import (
	"context"
	"encoding/json"
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

type mockTaskGetter struct {
	tasks []domain.Task
	total int
	err   error

	called    bool
	gotUserID int64
	gotLimit  int
	gotOffset int
}

func (m *mockTaskGetter) GetTasks(ctx context.Context, userID int64, limit, offset int) ([]domain.Task, int, error) {
	m.called = true
	m.gotUserID = userID
	m.gotLimit = limit
	m.gotOffset = offset
	return m.tasks, m.total, m.err
}

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)

	limit, offset, err := parsePagination(req)
	require.NoError(t, err)
	require.Equal(t, defaultLimit, limit)
	require.Equal(t, 0, offset)
}

func TestParsePagination_ClampLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=1000&offset=5", nil)

	limit, offset, err := parsePagination(req)
	require.NoError(t, err)
	require.Equal(t, maxLimit, limit)
	require.Equal(t, 5, offset)
}

func TestParsePagination_InvalidLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=0", nil)

	_, _, err := parsePagination(req)
	require.Error(t, err)
}

func TestParsePagination_InvalidOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks?offset=-1", nil)

	_, _, err := parsePagination(req)
	require.Error(t, err)
}

func TestNew_ReturnsPaginatedTasks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	getter := &mockTaskGetter{
		tasks: []domain.Task{
			{ID: 1, UserID: 42, Title: "Task 1", Status: domain.TaskStatusPending},
		},
		total: 25,
	}

	h := New(logger, getter)

	secret := "test-secret"
	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "user@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Get("/tasks", h)

	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=10&offset=20", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, getter.called)
	require.Equal(t, int64(42), getter.gotUserID)
	require.Equal(t, 10, getter.gotLimit)
	require.Equal(t, 20, getter.gotOffset)

	var resp struct {
		Status     string        `json:"status"`
		Tasks      []domain.Task `json:"tasks"`
		Pagination Pagination    `json:"pagination"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "OK", resp.Status)
	require.Len(t, resp.Tasks, 1)
	require.Equal(t, 25, resp.Pagination.Total)
	require.Equal(t, 10, resp.Pagination.Limit)
	require.Equal(t, 20, resp.Pagination.Offset)
}

func TestNew_RejectsInvalidPagination(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	getter := &mockTaskGetter{}

	h := New(logger, getter)

	secret := "test-secret"
	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "user@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Get("/tasks", h)

	req := httptest.NewRequest(http.MethodGet, "/tasks?limit=-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, getter.called)

	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "Error", resp.Status)
	require.Equal(t, "invalid pagination params", resp.Error)
}

func TestNew_UnauthorizedWithoutToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	getter := &mockTaskGetter{}

	h := New(logger, getter)

	router := chi.NewRouter()
	router.Use(authmw.New("secret"))
	router.Get("/tasks", h)

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.False(t, getter.called)

	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "Error", resp.Status)
	require.Equal(t, "unauthorized", resp.Error)
}

func TestNew_PropagatesStorageError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	getter := &mockTaskGetter{err: storage.ErrTaskNotFound}

	h := New(logger, getter)

	secret := "test-secret"
	token, err := jwt.NewAccessToken(domain.User{ID: 7, Email: "user@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(authmw.New(secret))
	router.Get("/tasks", h)

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}
