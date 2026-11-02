package refresh

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRefreshRejectsOversizedInputBeforeRotation(t *testing.T) {
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"refresh_token":"` + strings.Repeat("x", 8192) + `"}`, 413},
		{`{"refresh_token":"short"}` + strings.Repeat(" ", 8192), 413},
		{`{"refresh_token":"` + strings.Repeat("x", 65) + `"}`, 400},
		{`{"refresh_token":"` + strings.Repeat("я", 33) + `"}`, 400},
		{`{"refresh_token":"short"} {}`, 400},
	} {
		w := httptest.NewRecorder()
		New(slog.New(slog.DiscardHandler), nil, &config.Config{})(w, httptest.NewRequest("POST", "/refresh", strings.NewReader(test.body)))
		require.Equal(t, test.status, w.Code)
		require.NotContains(t, w.Body.String(), `"access_token"`)
	}
}
