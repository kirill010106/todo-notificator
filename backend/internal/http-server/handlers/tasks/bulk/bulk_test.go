package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockBulkStorage struct {
	mock.Mock
}

func (m *mockBulkStorage) BulkCompleteTasks(ctx context.Context, userID int64, taskIDs []int64) (int64, error) {
	args := m.Called(ctx, userID, taskIDs)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockBulkStorage) BulkDeleteTasks(ctx context.Context, userID int64, taskIDs []int64) (int64, error) {
	args := m.Called(ctx, userID, taskIDs)
	return args.Get(0).(int64), args.Error(1)
}

func TestBulkComplete_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockStorage := new(mockBulkStorage)

	secret := "secret"
	userID := int64(1)
	tok, err := jwt.NewAccessToken(domain.User{ID: userID, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	taskIDs := []int64{10, 20}
	mockStorage.On("BulkCompleteTasks", mock.Anything, userID, taskIDs).Return(int64(2), nil)

	handler := NewComplete(log, mockStorage)
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Post("/tasks/bulk-complete", handler)

	body, _ := json.Marshal(Request{TaskIDs: taskIDs})
	req := httptest.NewRequest(http.MethodPost, "/tasks/bulk-complete", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp Response
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
	assert.Equal(t, int64(2), resp.AffectedCount)
}

func TestBulkDelete_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockStorage := new(mockBulkStorage)

	secret := "secret"
	userID := int64(1)
	tok, err := jwt.NewAccessToken(domain.User{ID: userID, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	taskIDs := []int64{10, 20}
	mockStorage.On("BulkDeleteTasks", mock.Anything, userID, taskIDs).Return(int64(2), nil)

	handler := NewDelete(log, mockStorage)
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Post("/tasks/bulk-delete", handler)

	body, _ := json.Marshal(Request{TaskIDs: taskIDs})
	req := httptest.NewRequest(http.MethodPost, "/tasks/bulk-delete", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp Response
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
	assert.Equal(t, int64(2), resp.AffectedCount)
}

func TestBulk_EmptyBody(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockStorage := new(mockBulkStorage)

	secret := "secret"
	userID := int64(1)
	tok, err := jwt.NewAccessToken(domain.User{ID: userID, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	handler := NewDelete(log, mockStorage)
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Post("/tasks/bulk-delete", handler)

	req := httptest.NewRequest(http.MethodPost, "/tasks/bulk-delete", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
