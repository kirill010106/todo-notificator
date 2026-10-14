package getone

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

type getterStub struct {
	t     *testing.T
	err   error
	calls int
}

func (s *getterStub) GetCategory(_ context.Context, user, id int64) (domain.Category, error) {
	require.Equal(s.t, int64(42), user)
	require.Equal(s.t, int64(7), id)
	s.calls++
	return domain.Category{ID: 7, UserID: 42, Name: "Work"}, s.err
}
func TestGetCategoryOwnershipAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, id      string
		err           error
		authenticated bool
		want, calls   int
	}{
		{"success", "7", nil, true, 200, 1}, {"missing", "7", storage.ErrCategoryNotFound, true, 404, 1},
		{"storage failure", "7", errors.New("db failed"), true, 500, 1},
		{"invalid", "abc", nil, true, 400, 0}, {"zero", "0", nil, true, 400, 0},
		{"negative", "-1", nil, true, 400, 0}, {"anonymous", "7", nil, false, 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &getterStub{t: t, err: tc.err}
			req := httptest.NewRequest("GET", "/categories/"+tc.id, nil)
			route := chi.NewRouteContext()
			route.URLParams.Add("category_id", tc.id)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
			handler := New(slog.New(slog.DiscardHandler), stub)
			response := httptest.NewRecorder()
			if tc.authenticated {
				token, err := jwt.NewAccessToken(domain.User{ID: 42}, "jwt", time.Hour)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+token)
				auth.New("jwt")(handler).ServeHTTP(response, req)
			} else {
				handler(response, req)
			}
			require.Equal(t, tc.want, response.Code)
			require.Equal(t, tc.calls, stub.calls)
			if tc.want == 200 {
				var body Response
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				require.Equal(t, "Work", body.Category.Name)
			}
		})
	}
}
