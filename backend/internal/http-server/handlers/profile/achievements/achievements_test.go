package achievements

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

func (m *mockProvider) GetAchievements(ctx context.Context, userID int64) ([]domain.Achievement, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]domain.Achievement), args.Error(1)
}

func TestAchievementsHandler_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockP := new(mockProvider)

	secret := "test-secret"
	userID := int64(10)
	user := domain.User{ID: userID, Email: "gamer@test.com"}

	tok, err := jwt.NewAccessToken(user, secret, time.Hour)
	require.NoError(t, err)

	expectedAchievements := []domain.Achievement{
		{Code: "first_step", Title: "Первый шаг", Unlocked: true},
		{Code: "streak_week", Title: "Неделя огня", Unlocked: false},
	}
	mockP.On("GetAchievements", mock.Anything, userID).Return(expectedAchievements, nil)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/api/v1/me/achievements", New(log, mockP))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/achievements", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp Response
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
	assert.Len(t, resp.Achievements, 2)
	assert.True(t, resp.Achievements[0].Unlocked)
	assert.False(t, resp.Achievements[1].Unlocked)
}
