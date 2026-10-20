//go:build integration

package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	"github.com/rvinnie/yookassa-sdk-go/yookassa"
	"github.com/rvinnie/yookassa-sdk-go/yookassa/opts"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type releasePaymentTransport func(*http.Request) (*http.Response, error)

func (f releasePaymentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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

	t.Run("payment creation retries without locking the account and disabled sync stays pending", func(t *testing.T) {
		original := handler
		defer func() { handler = original }()
		transport := releasePaymentTransport(func(r *http.Request) (*http.Response, error) {
			probe, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			// A provider call must not run while the router holds the user's row lock.
			_, err := store.DB.ExecContext(probe, `UPDATE users SET email=email WHERE id=$1`, user)
			if err != nil {
				return nil, err
			}
			paymentID := "release-" + r.Header.Get("Idempotence-Key")[:20]
			body := fmt.Sprintf(`{"id":%q,"status":"pending","confirmation":{"type":"redirect","confirmation_url":"https://checkout.test/%s"}}`, paymentID, paymentID)
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		client := yookassa.NewClient("shop", "key", opts.WithHTTPClient(http.Client{Transport: transport, Timeout: 5 * time.Second}))
		handler = router.New(router.Config{Log: slog.New(slog.DiscardHandler), Storage: store, YooClient: client, AppConfig: &config.Config{AppSecret: "secret", ClientURL: "https://app.test"}})
		first := request("POST", "/payments/create", `{}`, "payment-retry", 200)
		again := request("POST", "/payments/create", `{}`, "payment-retry", 200)
		require.Equal(t, first, again)
		var count int
		require.NoError(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM payments WHERE user_id=$1`, user).Scan(&count))
		require.Equal(t, 1, count)
		pending := request("POST", "/payments/sync", `{}`, "", 200)
		require.Equal(t, "pending", pending["payment_status"])
		callback := httptest.NewRequest("POST", "/api/v1/webhooks/yookassa", strings.NewReader(`{"event":"payment.succeeded","object":{"id":"unverified"}}`))
		callback.RemoteAddr = "127.0.0.1:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, callback)
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
	})

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
	t.Run("reminder contract and profile limits", func(t *testing.T) {
		created := request("POST", "/tasks", `{"title":"Reminders","deadline":"2099-01-01T12:00:00Z","reminders":[{"offset_minutes":1640},{"offset_minutes":0}]}`, "reminder-create", 201)
		id := int64(created["id"].(float64))
		path := fmt.Sprintf("/tasks/%d", id)
		task := created["task"].(map[string]any)
		require.Len(t, task["reminders"], 2)
		require.NotContains(t, task, "reminder_at")
		require.NotContains(t, task, "is_notified")
		failed := request("PATCH", path, `{"reminders":[{"offset_minutes":0},{"offset_minutes":1},{"offset_minutes":2}]}`, "reminder-limit", 403)
		require.Equal(t, "REMINDER_LIMIT_REACHED", failed["code"])
		require.Contains(t, failed["field_errors"], "reminders")
		invalid := request("PATCH", path, `{"reminders":[{"offset_minutes":1.5}]}`, "reminder-invalid", 400)
		require.Contains(t, invalid["field_errors"], "reminders.0")
		request("PATCH", path, `{"reminder_at":null}`, "reminder-old", 400)
		request("POST", "/tasks", `{"title":"old","is_notified":false}`, "reminder-old-create", 400)
		request("PATCH", path, `{"reminders":null}`, "reminder-null", 400)
		request("PATCH", path, `{"reminders":[{"scheduled_at":"2099-01-01T00:00:00Z"}]}`, "reminder-absolute", 400)
		profile := request("GET", "/me/bootstrap", "", "", 200)["user"].(map[string]any)
		require.Equal(t, float64(2), profile["reminders_per_task_limit"])
		require.NotContains(t, profile, "reminders_limit")
		require.NoError(t, store.SetPremiumStatus(ctx, user, true))
		profile = request("GET", "/me/bootstrap", "", "", 200)["user"].(map[string]any)
		require.Nil(t, profile["reminders_per_task_limit"])
		request("PATCH", path, `{"reminders":[{"offset_minutes":0},{"offset_minutes":1},{"offset_minutes":2}]}`, "reminder-premium", 200)
		require.NoError(t, store.SetPremiumStatus(ctx, user, false))
		request("PATCH", path, `{"reminders":[],"deadline":null}`, "reminder-clear", 200)
	})

	t.Run("shop purchases replay and cancellation has no penalty", func(t *testing.T) {
		initial := request("GET", "/me/shop", "", "", 200)
		require.NotEmpty(t, initial["catalog"])
		imported := request("POST", "/me/shop/actions", `{"action":"import_legacy","legacy":{"coins":99999,"activeTheme":"violet"}}`, "shop-import", 200)
		coins := imported["wallet"].(map[string]any)["coins"]
		bought := request("POST", "/me/shop/actions", `{"action":"buy","item_id":"ocean"}`, "shop-buy", 200)
		replay := request("POST", "/me/shop/actions", `{"action":"buy","item_id":"ocean"}`, "shop-buy", 200)
		require.Equal(t, bought, replay)
		require.Equal(t, coins.(float64)-40, bought["wallet"].(map[string]any)["coins"])
		request("POST", "/me/shop/actions", `{"action":"buy","item_id":"xp_boost"}`, "shop-expensive", 409)
		request("POST", "/me/shop/actions", `{"action":"equip","item_id":"ocean"}`, "shop-equip", 200)
		request("POST", "/me/shop/actions", `{"action":"unknown"}`, "shop-invalid", 400)
		request("POST", "/me/shop/actions", `{"action":"buy","unknown":true}`, "shop-shape", 400)
		started := request("POST", "/pomodoros/start", `{}`, "cancel-start", 201)
		sessionID := int64(started["session"].(map[string]any)["id"].(float64))
		require.NotNil(t, started["focus"].(map[string]any)["cancel_until"])
		path := fmt.Sprintf("/pomodoros/%d/cancel", sessionID)
		cancelled := request("POST", path, `{}`, "cancel-early", 200)
		require.Equal(t, "idle", cancelled["focus"].(map[string]any)["phase"])
		require.Equal(t, float64(0), cancelled["rewards"].(map[string]any)["penalty_xp"])
		request("POST", path, `{}`, "cancel-again", 409)
		request("POST", "/pomodoros/nope/cancel", `{}`, "cancel-invalid", 400)
	})

	t.Run("account HTTP sessions profile changes and password recovery", func(t *testing.T) {
		accounts := router.New(router.Config{Log: slog.New(slog.DiscardHandler), Storage: store, AppConfig: &config.Config{
			AppSecret: "account-test-secret", AccessTokenTTL: time.Hour, RefreshTokenTTL: 24 * time.Hour,
		}})
		call := func(method, path, body, access string, status int) map[string]any {
			t.Helper()
			req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			if access != "" {
				req.Header.Set("Authorization", "Bearer "+access)
			}
			res := httptest.NewRecorder()
			accounts.ServeHTTP(res, req)
			require.Equal(t, status, res.Code, "%s %s: %s", method, path, res.Body.String())
			var result map[string]any
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
			return result
		}
		registration := `{"email":"account-http@test.local","password":"password123","display_name":"Initial name"}`
		session := call("POST", "/register", registration, "", 200)
		id := int64(session["user_id"].(float64))
		access, refresh := session["access_token"].(string), session["refresh_token"].(string)
		require.NotEmpty(t, access)
		require.NotEmpty(t, refresh)
		require.Equal(t, float64(3600), session["expires_in"])
		require.Equal(t, "EMAIL_EXISTS", call("POST", "/register", registration, "", 409)["code"])

		call("PATCH", "/me/profile", `{"display_name":" Updated name "}`, access, 200)
		call("PATCH", "/me/profile", `{"timezone":"Europe/Moscow"}`, access, 200)
		profile := call("GET", "/me/profile", "", access, 200)["profile"].(map[string]any)
		require.Equal(t, "Updated name", profile["display_name"])
		require.Equal(t, "Europe/Moscow", profile["timezone"])
		call("PATCH", "/me/profile", `{"display_name":""}`, access, 200)
		profile = call("GET", "/me/profile", "", access, 200)["profile"].(map[string]any)
		require.Empty(t, profile["display_name"])
		require.Equal(t, "Europe/Moscow", profile["timezone"])

		var verification string
		require.NoError(t, store.DB.QueryRowContext(ctx, `SELECT token FROM email_verification_tokens WHERE user_id=$1`, id).Scan(&verification))
		call("GET", "/verify?token="+verification, "", "", 200)
		call("GET", "/verify?token="+verification, "", "", 400)
		profile = call("GET", "/me/profile", "", access, 200)["profile"].(map[string]any)
		require.Equal(t, true, profile["is_verified"])

		call("POST", "/me/email", `{"current_password":"password123"}`, access, 400)
		call("POST", "/me/email", `{"email":"invalid","current_password":"password123"}`, access, 400)
		call("POST", "/me/email", `{"email":"changed-http@test.local","current_password":"wrong-password"}`, access, 400)
		changed := call("POST", "/me/email", `{"email":"changed-http@test.local","current_password":"password123"}`, access, 200)
		require.Equal(t, true, changed["changed"])
		call("GET", "/me/profile", "", access, 401)
		call("POST", "/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh), "", 401)
		access, refresh = changed["access_token"].(string), changed["refresh_token"].(string)
		unchanged := call("POST", "/me/email", `{"email":"changed-http@test.local","current_password":"password123"}`, access, 200)
		require.Equal(t, false, unchanged["changed"])
		require.NotContains(t, unchanged, "access_token")
		call("POST", "/me/password", `{"current_password":"password123","new_password":"short"}`, access, 400)
		changed = call("POST", "/me/password", `{"current_password":"password123","new_password":"password456"}`, access, 200)
		call("GET", "/me/profile", "", access, 401)
		call("POST", "/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh), "", 401)
		access, refresh = changed["access_token"].(string), changed["refresh_token"].(string)
		profile = call("GET", "/me/profile", "", access, 200)["profile"].(map[string]any)
		require.Equal(t, "changed-http@test.local", profile["email"])
		require.Equal(t, false, profile["is_verified"])

		unknown := call("POST", "/password/forgot", `{"email":"missing-http@test.local"}`, "", 202)
		known := call("POST", "/password/forgot", `{"email":"changed-http@test.local"}`, "", 202)
		require.Equal(t, unknown, known)
		var reset string
		require.NoError(t, store.DB.QueryRowContext(ctx, `SELECT token FROM password_reset_outbox WHERE user_id=$1 AND status='scheduled'`, id).Scan(&reset))
		call("POST", "/password/reset", `{"token":"invalid-reset","new_password":"password789"}`, "", 400)
		body := fmt.Sprintf(`{"token":%q,"new_password":"password789"}`, reset)
		call("POST", "/password/reset", body, "", 200)
		call("POST", "/password/reset", body, "", 400)
		call("GET", "/me/profile", "", access, 401)
		call("POST", "/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh), "", 401)
		call("POST", "/login", `{"email":"changed-http@test.local","password":"password456"}`, "", 401)
		login := call("POST", "/login", `{"email":"changed-http@test.local","password":"password789"}`, "", 200)
		profile = call("GET", "/me/profile", "", login["access_token"].(string), 200)["profile"].(map[string]any)
		require.Equal(t, false, profile["is_verified"])
		var pendingSecrets int
		require.NoError(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_reset_outbox WHERE user_id=$1 AND token IS NOT NULL`, id).Scan(&pendingSecrets))
		require.Zero(t, pendingSecrets)
	})

}
