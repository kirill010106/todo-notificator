//go:build integration

package router_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	app "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type errorBaselineState struct {
	handler     *chi.Mux
	user        int64
	token, path string
	key         string
}

func (s *errorBaselineState) request(method, path, body string) *httptest.ResponseRecorder {
	req := baselineRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+s.token)
	if s.key != "" {
		req.Header.Set("Idempotency-Key", s.key)
	}
	response := httptest.NewRecorder()
	s.handler.ServeHTTP(response, req)
	return response
}

func TestErrorContractPostgresBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	require.NoError(t, err, "Docker is required with -tags integration")
	require.NoError(t, provider.Health(ctx))
	container, err := pg.Run(ctx, "postgres:16-alpine", pg.WithDatabase("errorbaseline"), pg.WithUsername("postgres"), pg.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	store, err := postgres.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	goose.SetBaseFS(app.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(store.DB, "migrations"))

	cases := []struct {
		name, method, path, body string
		setup                    func(*testing.T, *errorBaselineState)
		check                    func(*testing.T, *errorBaselineState)
	}{
		{name: "auth_account_missing", method: "GET", path: "/api/v1/tasks", setup: func(t *testing.T, s *errorBaselineState) {
			_, err := store.DB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, s.user)
			require.NoError(t, err)
		}},
		{name: "auth_version_changed", method: "GET", path: "/api/v1/tasks", setup: func(t *testing.T, s *errorBaselineState) {
			_, err := store.DB.ExecContext(ctx, `UPDATE users SET auth_version=auth_version+1 WHERE id=$1`, s.user)
			require.NoError(t, err)
		}},
		{name: "register_duplicate", method: "POST", path: "/api/v1/register", body: `{"email":"baseline-duplicate@example.invalid","password":"Password123!"}`, setup: func(t *testing.T, s *errorBaselineState) {
			_, err := store.SaveUser(ctx, "baseline-duplicate@example.invalid", []byte("unused-test-hash"))
			require.NoError(t, err)
		}},
		{name: "verify_invalid_token", method: "GET", path: "/api/v1/verify?token=invalid"},
		{name: "profile_empty", method: "PATCH", path: "/api/v1/me/profile", body: `{}`},
		{name: "today_invalid_pagination", method: "GET", path: "/api/v1/me/today?limit=0"},
		{name: "tasks_invalid_pagination", method: "GET", path: "/api/v1/tasks?limit=0"},
		{name: "category_missing", method: "GET", path: "/api/v1/categories/999999"},
		{name: "category_limit", method: "POST", path: "/api/v1/categories", body: `{"name":"Fourth"}`, setup: func(t *testing.T, s *errorBaselineState) {
			for _, name := range []string{"One", "Two", "Three"} {
				_, err := store.CreateCategory(ctx, domain.Category{UserID: s.user, Name: name})
				require.NoError(t, err)
			}
		}},
		{name: "category_duplicate", method: "POST", path: "/api/v1/categories", body: `{"name":"Existing"}`, setup: func(t *testing.T, s *errorBaselineState) {
			_, err := store.CreateCategory(ctx, domain.Category{UserID: s.user, Name: "Existing"})
			require.NoError(t, err)
		}},
		{name: "command_key_limit", method: "POST", path: "/api/v1/tasks", body: `{"title":"Task"}`, setup: func(t *testing.T, s *errorBaselineState) { s.key = strings.Repeat("x", 129) }},
		{name: "command_body_limit", method: "POST", path: "/api/v1/tasks", body: strings.Repeat("x", (1<<20)+1)},
		{name: "command_malformed_task", method: "POST", path: "/api/v1/tasks", body: "{"},
		{name: "command_legacy_reminder", method: "POST", path: "/api/v1/tasks", body: `{"title":"Task","reminder_at":null}`},
		{name: "command_reminders_null", method: "POST", path: "/api/v1/tasks", body: `{"title":"Task","reminders":null}`},
		{name: "command_reminder_field", method: "POST", path: "/api/v1/tasks", body: `{"title":"Task","reminders":[{"offset_minutes":1.5}]}`},
		{name: "command_title_validation", method: "POST", path: "/api/v1/tasks", body: `{}`},
		{name: "command_reminder_limit", method: "POST", path: "/api/v1/tasks", body: `{"title":"Task","deadline":"2099-01-01T12:00:00Z","reminders":[{"offset_minutes":0},{"offset_minutes":1},{"offset_minutes":2}]}`, setup: func(t *testing.T, s *errorBaselineState) {
			s.key = "baseline-reminder-limit"
		}, check: func(t *testing.T, s *errorBaselineState) {
			// SaveTask inserts the task before validating reminder policy. The
			// failure must roll back the task, its trigger fact and the operation.
			var tasks, facts, operations int
			err := store.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM tasks WHERE user_id=$1),(SELECT COUNT(*) FROM daily_task_activity WHERE user_id=$1),(SELECT COUNT(*) FROM api_operations WHERE user_id=$1)`, s.user).Scan(&tasks, &facts, &operations)
			require.NoError(t, err)
			require.Zero(t, tasks)
			require.Zero(t, facts)
			require.Zero(t, operations)
		}},
		{name: "command_task_missing", method: "PATCH", path: "/api/v1/tasks/999999", body: `{"title":"Changed"}`},
		{name: "command_key_conflict", method: "POST", path: "/api/v1/tasks", body: `{"title":"Other"}`, setup: func(t *testing.T, s *errorBaselineState) {
			s.key = "baseline-used-key"
			created := s.request("POST", "/api/v1/tasks", `{"title":"Original"}`)
			require.Equal(t, 201, created.Code, created.Body.String())
		}},
		{name: "pause_exhausted", method: "POST", setup: func(t *testing.T, s *errorBaselineState) {
			session, err := store.StartPomodoroSession(ctx, s.user, nil)
			require.NoError(t, err)
			require.NoError(t, store.AddPomodoroBreak(ctx, s.user, session.ID))
			s.path = fmt.Sprintf("/api/v1/pomodoros/%d/pause", session.ID)
		}},
		{name: "shop_malformed", method: "POST", path: "/api/v1/me/shop/actions", body: `{"unknown":true}`},
		{name: "shop_unknown_action", method: "POST", path: "/api/v1/me/shop/actions", body: `{"action":"unknown"}`},
		{name: "cancel_invalid_id", method: "POST", path: "/api/v1/pomodoros/invalid/cancel"},
		{name: "cancel_session_missing", method: "POST", path: "/api/v1/pomodoros/999999/cancel"},
		{name: "payments_disabled", method: "POST", path: "/api/v1/payments/create", body: `{}`},
		{name: "user_rate_limit", method: "GET", path: "/api/v1/tasks?limit=0", setup: func(t *testing.T, s *errorBaselineState) {
			for i := 0; i < 100; i++ {
				response := s.request("GET", s.path, "")
				require.Equal(t, 400, response.Code, "request %d: %s", i+1, response.Body.String())
			}
		}},
	}
	fixtures := loadErrorBaseline(t, "testdata/error_contract_postgres_baseline.json", len(cases))
	seen := map[string]bool{}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, seen[tc.name], "duplicate scenario")
			seen[tc.name] = true
			user, err := store.SaveUser(ctx, fmt.Sprintf("error-baseline-%d@example.invalid", i), []byte("unused-test-hash"))
			require.NoError(t, err)
			token, err := jwt.NewAccessToken(domain.User{ID: user}, baselineSecret, time.Hour)
			require.NoError(t, err)
			state := &errorBaselineState{handler: router.New(baselineConfig(store)), user: user, token: token, path: tc.path}
			if tc.setup != nil {
				tc.setup(t, state)
			}
			assertErrorBaseline(t, fixtures, tc.name, state.request(tc.method, state.path, tc.body))
			if tc.check != nil {
				tc.check(t, state)
			}
		})
	}
}
