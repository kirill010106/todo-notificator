package logout

import (
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogoutAllDevicesAndValidation(t *testing.T) {
	for _, tc := range []struct {
		body string
		err  error
		want int
	}{
		{`{"refresh_token":"token","all_devices":true}`, nil, 204},
		{`{"refresh_token":"token","all_devices":true}`, errors.New("db failed"), 500},
		{"", nil, 400}, {`{}`, nil, 400},
	} {
		t.Run(tc.body+string(rune(tc.want)), func(t *testing.T) {
			token, err := jwt.NewAccessToken(domain.User{ID: 42}, "jwt", time.Hour)
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/logout", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			auth.New("jwt")(New(slog.New(slog.DiscardHandler), mockRevoker{allErr: tc.err})).ServeHTTP(res, req)
			require.Equal(t, tc.want, res.Code, res.Body.String())
		})
	}
}
