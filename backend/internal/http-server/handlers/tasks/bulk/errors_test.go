package bulk

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type failingStorage struct{ calls int }

func (s *failingStorage) BulkCompleteTasks(context.Context, int64, []int64) (int64, error) {
	s.calls++
	return 0, errors.New("db unavailable")
}
func (s *failingStorage) BulkDeleteTasks(context.Context, int64, []int64) (int64, error) {
	s.calls++
	return 0, errors.New("db unavailable")
}
func TestBulkRejectsBadInputAndPropagatesStorageFailures(t *testing.T) {
	for _, constructor := range []struct {
		name string
		new  func(*slog.Logger, BulkTaskStorage) http.HandlerFunc
	}{{"complete", NewComplete}, {"delete", NewDelete}} {
		t.Run(constructor.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, body  string
				authorized  bool
				want, calls int
			}{
				{"anonymous", `{"task_ids":[1]}`, false, 401, 0},
				{"empty", "", true, 400, 0}, {"malformed", "{", true, 400, 0},
				{"empty list", `{"task_ids":[]}`, true, 400, 0},
				{"storage error", `{"task_ids":[1,2]}`, true, 500, 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					store := &failingStorage{}
					handler := constructor.new(slog.New(slog.DiscardHandler), store)
					req := httptest.NewRequest("POST", "/tasks/bulk-"+constructor.name, strings.NewReader(tc.body))
					res := httptest.NewRecorder()
					if tc.authorized {
						token, err := jwt.NewAccessToken(domain.User{ID: 42}, "jwt", time.Hour)
						require.NoError(t, err)
						req.Header.Set("Authorization", "Bearer "+token)
						auth.New("jwt")(handler).ServeHTTP(res, req)
					} else {
						handler(res, req)
					}
					require.Equal(t, tc.want, res.Code, res.Body.String())
					require.Equal(t, tc.calls, store.calls)
					require.NotContains(t, res.Body.String(), `"status":"OK"`)
				})
			}
		})
	}
}
