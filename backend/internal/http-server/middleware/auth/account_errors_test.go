package auth

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type accountFailure struct{ err error }

func (a accountFailure) GetUserByID(context.Context, int64) (*domain.User, error) { return nil, a.err }
func TestAccountLookupFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"deleted", storage.ErrUserNotFound, 401}, {"database unavailable", errors.New("db"), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwt.NewAccessToken(domain.User{ID: 42, IsPremium: true, IsVerified: true}, "jwt", time.Hour)
			require.NoError(t, err)
			req := httptest.NewRequest("GET", "/tasks", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			New("jwt", accountFailure{tc.err})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("authorized after account lookup failure") })).ServeHTTP(res, req)
			require.Equal(t, tc.want, res.Code)
		})
	}
}
