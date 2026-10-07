package quests

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

type mockProvider struct {
	mock.Mock
}

func (m *mockProvider) GetDailyQuests(ctx context.Context, userID int64) ([]domain.DailyQuest, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]domain.DailyQuest), args.Error(1)
}

func TestQuestsHandler_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockP := new(mockProvider)

	secret := "test-secret"
	userID := int64(10)
	user := domain.User{ID: userID, Email: "gamer@test.com"}

	tok, err := jwt.NewAccessToken(user, secret, time.Hour)
	require.NoError(t, err)

	expectedQuests := []domain.DailyQuest{
		{Code: "create_task", Title: "Планировщик", Target: 1, Current: 1, Completed: true, RewardXP: 10},
		{Code: "pomodoro_focus", Title: "Глубокий фокус", Target: 1, Current: 0, Completed: false, RewardXP: 20},
	}
	mockP.On("GetDailyQuests", mock.Anything, userID).Return(expectedQuests, nil)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/api/v1/me/quests", New(log, mockP))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/quests", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp Response
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
	assert.Len(t, resp.Quests, 2)
	assert.True(t, resp.Quests[0].Completed)
	assert.False(t, resp.Quests[1].Completed)
}
