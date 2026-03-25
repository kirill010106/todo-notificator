package get

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
)

type mockUserStatsGetter struct {
	stats  domain.UserStats
	err    error
	called bool
}

func (m *mockUserStatsGetter) GetUserStats(ctx context.Context, userID int64) (domain.UserStats, error) {
	m.called = true
	return m.stats, m.err
}

func TestStatsHandler_Success(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)

	getter := &mockUserStatsGetter{
		stats: domain.UserStats{
			ID:              1,
			UserID:          42,
			Points:          nil,
			Level:           nil,
			TotalPomodoros:  nil,
			TotalBurntTasks: nil,
			CurrentStreak:   nil,
			BestStreak:      nil,
			UpdatedAt:       func() *time.Time { tm := time.Now(); return &tm }(),
		},
	}
	log := slog.New(slog.DiscardHandler)
	h := New(log, getter)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/stats", h)

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, getter.called)
	require.Contains(t, w.Body.String(), "user_stats")
}

func TestStatsHandler_Error(t *testing.T) {
	secret := "secret"
	tok, err := jwt.NewAccessToken(domain.User{ID: 5, Email: "u@test.com"}, secret, time.Hour)
	require.NoError(t, err)
	getter := &mockUserStatsGetter{
		err: context.DeadlineExceeded,
	}
	log := slog.New(slog.DiscardHandler)
	h := New(log, getter)

	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/stats", h)

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.True(t, getter.called)
	require.Contains(t, w.Body.String(), "failed to get user stats")
}
