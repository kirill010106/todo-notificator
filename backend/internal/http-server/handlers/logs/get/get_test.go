package get_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/logs/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/logs/get/mocks"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetLogsHandler(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	secret := "test-secret"
	tokenStr, err := jwt.NewAccessToken(domain.User{ID: 1, Email: "test@example.com"}, secret, time.Hour)
	require.NoError(t, err)

	now := time.Now()

	tests := []struct {
		name           string
		userIDParam    string
		limitQuery     string
		offsetQuery    string
		mockSetup      func(*mocks.LogsGetter)
		expectedStatus int
	}{
		{
			name:        "Success",
			userIDParam: "1",
			limitQuery:  "10",
			offsetQuery: "5",
			mockSetup: func(m *mocks.LogsGetter) {
				m.On("GetLogs", mock.Anything, int64(1), int32(10), int32(5)).Return(
					[]domain.ActivityLog{
						{
							ID:        "id1",
							UserID:    1,
							Action:    "TASK_CREATED",
							Timestamp: now,
						},
					}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Empty logs",
			userIDParam: "1",
			mockSetup: func(m *mocks.LogsGetter) {
				m.On("GetLogs", mock.Anything, int64(1), int32(50), int32(0)).Return(
					nil, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "Database error",
			userIDParam: "1",
			mockSetup: func(m *mocks.LogsGetter) {
				m.On("GetLogs", mock.Anything, int64(1), int32(50), int32(0)).Return(
					nil, errors.New("db string error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewLogsGetter(t)
			tt.mockSetup(m)

			handler := get.New(log, m)

			r := chi.NewRouter()
			r.Use(authmw.New(secret))
			r.Get("/logs/{userId}", handler)

			reqURL := "/logs/" + tt.userIDParam
			if tt.limitQuery != "" || tt.offsetQuery != "" {
				reqURL += "?"
				if tt.limitQuery != "" {
					reqURL += "limit=" + tt.limitQuery + "&"
				}
				if tt.offsetQuery != "" {
					reqURL += "offset=" + tt.offsetQuery
				}
			}

			req, err := http.NewRequest("GET", reqURL, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+tokenStr)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			require.Equal(t, tt.expectedStatus, rr.Code)

			m.AssertExpectations(t)
		})
	}
}
