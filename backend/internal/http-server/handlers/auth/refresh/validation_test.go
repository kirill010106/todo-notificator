package refresh

import (
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidRequestCannotRotateTokens(t *testing.T) {
	for _, body := range []string{"", "{", `{}`, `{"refresh_token":42}`} {
		res := httptest.NewRecorder()
		// A nil provider makes an accidental rotation fail this test immediately.
		New(slog.New(slog.DiscardHandler), nil, &config.Config{})(res, httptest.NewRequest("POST", "/refresh", strings.NewReader(body)))
		require.Equal(t, 400, res.Code)
		require.NotContains(t, res.Body.String(), `"access_token"`)
	}
}
