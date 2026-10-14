//go:build integration

package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/config"
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

// Exercises the real router, auth, command middleware and storage together.
func TestAPIContracts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	require.NoError(t, err, "Docker is required with -tags integration")
	require.NoError(t, provider.Health(ctx))
	container, err := pg.Run(ctx, "postgres:16-alpine", pg.WithDatabase("httpcontracts"), pg.WithUsername("postgres"), pg.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	store, err := postgres.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	goose.SetBaseFS(app.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(store.DB, "migrations"))
	user, err := store.SaveUser(ctx, "http@test.local", []byte("test"))
	require.NoError(t, err)
	_, err = store.DB.ExecContext(ctx, `UPDATE users SET is_verified=true WHERE id=$1`, user)
	require.NoError(t, err)
	// The token deliberately retains the old unverified flag.
	token, err := jwt.NewAccessToken(domain.User{ID: user}, "secret", time.Hour)
	require.NoError(t, err)
	handler := router.New(router.Config{Log: slog.New(slog.DiscardHandler), Storage: store, AppConfig: &config.Config{AppSecret: "secret"}})
	request := func(method, path, body, key string, expected int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, expected, res.Code, res.Body.String())
		var payload map[string]any
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
		return payload
	}
	idle := request("GET", "/me/focus", "", "", 200)
	require.Equal(t, "idle", idle["focus"].(map[string]any)["phase"])
	created := request("POST", "/tasks", `{"title":"HTTP task","deadline":"2099-01-01T00:00:00Z"}`, "create", 201)
	replay := request("POST", "/tasks", `{"deadline":"2099-01-01T00:00:00Z","title":"HTTP task"}`, "create", 201)
	require.Equal(t, created, replay)
	id := int64(created["id"].(float64))
	updated := request("PATCH", fmt.Sprintf("/tasks/%d", id), `{"deadline":null,"status":"done"}`, "complete", 200)
	require.Nil(t, updated["task"].(map[string]any)["deadline"])
	require.Equal(t, float64(10), updated["rewards"].(map[string]any)["task_xp"])
	require.Equal(t, "idle", updated["focus"].(map[string]any)["phase"])
	conflict := request("POST", "/tasks", `{"title":"Other"}`, "create", 409)
	require.Equal(t, "IDEMPOTENCY_KEY_REUSED", conflict["code"])
	require.NotEmpty(t, conflict["request_id"])
	require.Contains(t, conflict, "field_errors")
	request("PATCH", "/me/stats", `{"points":99999}`, "", 405)
	request("PATCH", "/me/profile", `{"timezone":"Europe/Moscow"}`, "", 200)
	today := request("GET", "/me/today", "", "", 200)
	require.Equal(t, "Europe/Moscow", today["timezone"])
	claim := request("POST", "/me/quests/create_task/claim", `{"points":99999}`, "claim", 200)
	require.Equal(t, float64(10), claim["rewards"].(map[string]any)["quest_xp"])
	request("POST", "/me/quests/create_task/claim", `{}`, "claim-again", 409)
	bootstrap := request("GET", "/me/bootstrap", "", "", 200)
	require.Equal(t, "Europe/Moscow", bootstrap["user"].(map[string]any)["timezone"])
	require.Contains(t, bootstrap, "focus")

	t.Run("invalid contracts do not mutate state", func(t *testing.T) {
		for _, tc := range []struct {
			method, path, body string
			status             int
		}{
			{"GET", "/me/today?limit=0", "", 400},
			{"GET", "/me/today?limit=101", "", 400},
			{"GET", "/me/today?today_offset=-1", "", 400},
			{"GET", "/me/today?overdue_offset=abc", "", 400},
			{"GET", "/me/today?timezone=not-a-zone", "", 400},
			{"PATCH", "/me/profile", "{", 400},
			{"PATCH", "/me/profile", `{}`, 400},
			{"PATCH", "/me/profile", `{"timezone":"not-a-zone"}`, 400},
			{"POST", "/me/quests/unknown/claim", `{}`, 404},
			{"POST", "/me/rewards/daily-chest/claim", `{}`, 409},
			{"POST", "/me/focus/resume", `{}`, 409},
			{"POST", "/tasks", "", 400},
			{"POST", "/tasks", "{", 400},
			{"POST", "/tasks", `{"title":""}`, 400},
			{"PATCH", fmt.Sprintf("/tasks/%d", id), "", 400},
			{"PATCH", fmt.Sprintf("/tasks/%d", id), "{", 400},
			{"PATCH", fmt.Sprintf("/tasks/%d", id), `{"status":"invalid"}`, 400},
		} {
			payload := request(tc.method, tc.path, tc.body, "", tc.status)
			require.NotEmpty(t, payload["code"])
			require.NotEmpty(t, payload["request_id"])
		}
	})
	t.Run("focus commands return persisted phases", func(t *testing.T) {
		started := request("POST", "/pomodoros/start", `{}`, "focus-start", 201)
		sessionID := int64(started["session"].(map[string]any)["id"].(float64))
		request("POST", "/pomodoros/start", `{}`, "focus-conflict", 409)
		paused := request("POST", fmt.Sprintf("/pomodoros/%d/pause", sessionID), "", "pause", 200)
		require.Equal(t, "extra_break", paused["focus"].(map[string]any)["phase"])
		resumed := request("POST", "/me/focus/resume", "", "resume", 200)
		require.Equal(t, "focus", resumed["focus"].(map[string]any)["phase"])
		_, err := store.DB.ExecContext(ctx, `UPDATE pomodoro_sessions SET started_at=CURRENT_TIMESTAMP-INTERVAL '25 minutes' WHERE id=$1`, sessionID)
		require.NoError(t, err)
		finished := request("POST", fmt.Sprintf("/pomodoros/%d/stop", sessionID), `{"action":"completed","finish_task":false}`, "finish", 200)
		require.Equal(t, "recovery_break", finished["focus"].(map[string]any)["phase"])
		idle := request("POST", "/me/focus/rest/finish", "", "rest-finish", 200)
		require.Equal(t, "idle", idle["focus"].(map[string]any)["phase"])
	})
}
