package register

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestRegister(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		mockID     int64
		mockErr    error
		wantCode   int
		wantStatus string
	}{
		{
			name:       "happy path",
			body:       `{"email":"test@test.com", "password": "password123"}`,
			mockID:     1,
			mockErr:    nil,
			wantCode:   http.StatusOK,
			wantStatus: "OK",
		},
		{
			name:       "user already exists",
			body:       `{"email":"test@test.com", "password": "password123"}`,
			mockID:     0,
			mockErr:    storage.ErrUserExists,
			wantCode:   http.StatusBadRequest,
			wantStatus: "Error",
		},
		{
			name:       "invalid email",
			body:       `{"email":"notanemail", "password": "password123"}`,
			mockID:     0,
			mockErr:    nil,
			wantCode:   http.StatusBadRequest,
			wantStatus: "Error",
		},
		{
			name:       "empty body",
			body:       `{}`,
			mockID:     0,
			mockErr:    nil,
			wantCode:   http.StatusBadRequest,
			wantStatus: "Error",
		},
		{
			name:       "db error",
			body:       `{"email":"test@test.com", "password": "password123"}`,
			mockID:     0,
			mockErr:    errors.New("db error"),
			wantCode:   http.StatusInternalServerError,
			wantStatus: "Error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Arrange
			mock := &mockUserSaver{ID: tt.mockID, Error: tt.mockErr}
			handler := New(slog.New(slog.DiscardHandler), mock)
			req := httptest.NewRequest("POST", "/register", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			//Act
			handler(w, req)

			//Assert

			var response Response
			err := json.NewDecoder(w.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCode, w.Code)
			assert.Equal(t, tt.wantStatus, response.Response.Status)
		})
	}
}
