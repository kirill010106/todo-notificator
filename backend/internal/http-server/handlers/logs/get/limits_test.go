package get_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/logs/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/logs/get/mocks"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHistoryPageIsBoundedForEachPlan(t *testing.T) {
	for _, test := range []struct {
		premium       bool
		query         string
		limit, offset int32
	}{
		{true, "2147483647", 100, 7}, {true, "101", 100, 7}, {true, "100", 100, 7},
		{true, "-1", 50, 7}, {true, "2147483648", 50, 7},
		{false, "2147483647", 5, 0},
	} {
		m := mocks.NewLogsGetter(t)
		m.On("GetLogs", mock.Anything, int64(1), test.limit, test.offset).Return([]domain.ActivityLog{}, nil).Once()
		token, err := jwt.NewAccessToken(domain.User{ID: 1, IsPremium: test.premium}, "test", time.Hour)
		require.NoError(t, err)
		r := httptest.NewRequest("GET", "/logs?limit="+test.query+"&offset=7", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		auth.New("test")(get.New(slog.New(slog.NewTextHandler(io.Discard, nil)), m)).ServeHTTP(w, r)
		require.Equal(t, 200, w.Code)
	}
}
