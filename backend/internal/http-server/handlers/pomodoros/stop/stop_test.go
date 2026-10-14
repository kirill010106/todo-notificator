package stop

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

type mockPomodoroProvider struct {
	err               error
	called            bool
	userID, sessionID int64
	action            string
	finishTask        bool
}

func (m *mockPomodoroProvider) FinishPomodoroSession(ctx context.Context, userID, sessionID int64, action string, finishTask bool) error {
	m.called, m.userID, m.sessionID, m.action, m.finishTask = true, userID, sessionID, action, finishTask
	return m.err
}
func TestStop_TransactionalAction(t *testing.T) {
	cases := []struct {
		name, id, body string
		providerErr    error
		want           int
		called         bool
	}{
		{"complete", "10", `{"action":"completed","finish_task":true}`, nil, 200, true},
		{"abandon", "10", `{"action":"abandoned"}`, nil, 200, true},
		{"missing session", "10", `{"action":"completed"}`, storage.ErrSessionNotFound, 404, true},
		{"foreign task", "10", `{"action":"completed"}`, storage.ErrTaskNotFound, 404, true},
		{"missing user", "10", `{"action":"completed"}`, storage.ErrUserNotFound, 401, true},
		{"transaction failure", "10", `{"action":"completed"}`, errors.New("database failed"), 500, true},
		{"bad id", "0", `{"action":"completed"}`, nil, 400, false},
		{"bad action", "10", `{"action":"anything"}`, nil, 400, false},
		{"empty", "10", "", nil, 400, false},
		{"bad JSON", "10", "{", nil, 400, false},
	}
	token, err := jwt.NewAccessToken(domain.User{ID: 5}, "secret", time.Hour)
	require.NoError(t, err)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &mockPomodoroProvider{err: tc.providerErr}
			router := chi.NewRouter()
			router.Use(authmw.New("secret"))
			router.Post("/pomodoros/{id}/stop", New(slog.New(slog.DiscardHandler), provider, nil))
			req := httptest.NewRequest(http.MethodPost, "/pomodoros/"+tc.id+"/stop", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, tc.want, recorder.Code)
			require.Equal(t, tc.called, provider.called)
			if tc.called {
				require.EqualValues(t, 5, provider.userID)
				require.EqualValues(t, 10, provider.sessionID)
				if tc.name == "complete" {
					require.Equal(t, "completed", provider.action)
					require.True(t, provider.finishTask)
				}
			}
		})
	}
}
func TestStop_Unauthorized(t *testing.T) {
	provider := &mockPomodoroProvider{}
	recorder := httptest.NewRecorder()
	New(slog.New(slog.DiscardHandler), provider, nil).ServeHTTP(recorder,
		httptest.NewRequest(http.MethodPost, "/pomodoros/10/stop", strings.NewReader(`{"action":"completed"}`)))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.False(t, provider.called)
}
