package bootstrap

import (
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

type mockDataProvider struct {
	mock.Mock
}

func (m *mockDataProvider) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockDataProvider) GetCategories(ctx context.Context, userID int64) ([]domain.Category, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]domain.Category), args.Error(1)
}

func (m *mockDataProvider) GetTasks(ctx context.Context, userID int64, limit, offset int, filter domain.TaskFilter) ([]domain.Task, int, error) {
	args := m.Called(ctx, userID, limit, offset, filter)
	return args.Get(0).([]domain.Task), args.Int(1), args.Error(2)
}

func (m *mockDataProvider) GetUserStats(ctx context.Context, userID int64) (domain.UserStats, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(domain.UserStats), args.Error(1)
}

func (m *mockDataProvider) GetGamificationProfile(ctx context.Context, userID int64) (domain.GamificationProfile, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(domain.GamificationProfile), args.Error(1)
}

func (m *mockDataProvider) GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PomodoroSession), args.Error(1)
}

func (m *mockDataProvider) CountCategories(ctx context.Context, userID int64) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *mockDataProvider) CountActiveReminders(ctx context.Context, userID int64) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func TestBootstrap_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockP := new(mockDataProvider)

	secret := "secret"
	userID := int64(42)
	user := &domain.User{
		ID:         userID,
		Email:      "test@example.com",
		IsVerified: true,
		IsPremium:  false,
	}

	tok, err := jwt.NewAccessToken(*user, secret, time.Hour)
	require.NoError(t, err)

	points := int64(100)
	mockP.On("GetUserByID", mock.Anything, userID).Return(user, nil)
	mockP.On("GetCategories", mock.Anything, userID).Return([]domain.Category{{ID: 1, Name: "Work"}}, nil)
	mockP.On("GetTasks", mock.Anything, userID, 20, 0, domain.TaskFilter{}).Return([]domain.Task{{ID: 10, Title: "Task 1"}}, 1, nil)
	mockP.On("GetUserStats", mock.Anything, userID).Return(domain.UserStats{Points: &points}, nil)
	mockP.On("GetGamificationProfile", mock.Anything, userID).Return(domain.GamificationProfile{Level: 1, Points: 100, RankTitle: "Новичок"}, nil)
	mockP.On("GetActivePomodoroSession", mock.Anything, userID).Return((*domain.PomodoroSession)(nil), nil)
	mockP.On("CountCategories", mock.Anything, userID).Return(1, nil)
	mockP.On("CountActiveReminders", mock.Anything, userID).Return(0, nil)

	handler := New(log, mockP)
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/api/v1/me/bootstrap", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp Response
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
	assert.Equal(t, "test@example.com", resp.User.Email)
	assert.Equal(t, 3, resp.User.CategoriesLimit)
	assert.Len(t, resp.Categories, 1)
	assert.Len(t, resp.Tasks, 1)
	assert.Equal(t, int64(100), *resp.Stats.Points)
}
