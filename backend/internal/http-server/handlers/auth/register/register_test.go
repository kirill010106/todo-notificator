package register

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/assert"
)

// SaveUser(ctx context.Context, email string, passHash []byte) (int64, error)
type mockUserSaver struct {
	ID    int64
	Error error
}

func (m *mockUserSaver) SaveUser(ctx context.Context, email string, passHash []byte) (int64, error) {
	return m.ID, m.Error
}

func TestRegister_HappyPath(t *testing.T) {

	mock := &mockUserSaver{
		ID:    1,
		Error: nil,
	}

	handler := New(slog.New(slog.DiscardHandler), mock)

	body := bytes.NewBufferString(
		`{"email":"test@test.com", "password": "password123"}`,
	)

	req := httptest.NewRequest("POST", "/register", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	var response Response

	err := json.NewDecoder(w.Body).Decode(&response)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(1), response.UserID)
}

func TestRegister_UserAlreadyExists(t *testing.T) {
	mock := &mockUserSaver{
		ID:    0,
		Error: storage.ErrUserExists,
	}

	handler := New(slog.New(slog.DiscardHandler), mock)

	body := bytes.NewBufferString(
		`{"email":"test@test.com", "password": "password123"}`,
	)

	req := httptest.NewRequest("POST", "/register", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	var response Response

	err := json.NewDecoder(w.Body).Decode(&response)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "Error", response.Response.Status)
	assert.Equal(t, int64(0), response.UserID)

}
