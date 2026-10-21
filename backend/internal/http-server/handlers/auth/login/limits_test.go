package login

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoginRejectsOversizedInputBeforeStorage(t *testing.T) {
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"email":"user@example.com","password":"` + strings.Repeat("x", 8192) + `"}`, 413},
		{`{"email":"user@example.com","password":"password123"}` + strings.Repeat(" ", 8192), 413},
		{`{"email":"user@example.com","password":"` + strings.Repeat("x", 73) + `"}`, 400},
		{`{"email":"user@example.com","password":"` + strings.Repeat("я", 37) + `"}`, 400},
		{`{"email":"` + strings.Repeat("a", 255) + `@example.com","password":"password123"}`, 400},
		{`{"email":"user@example.com","password":"password123"} {}`, 400},
	} {
		w := httptest.NewRecorder()
		// A nil provider guarantees invalid input never reaches account lookup.
		New(slog.New(slog.DiscardHandler), nil, &config.Config{}, nil)(w, httptest.NewRequest("POST", "/login", strings.NewReader(test.body)))
		require.Equal(t, test.status, w.Code)
		require.NotContains(t, w.Body.String(), `"access_token"`)
	}
}
