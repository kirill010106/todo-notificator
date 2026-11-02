package router_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"
	"github.com/rvinnie/yookassa-sdk-go/yookassa/opts"
	"github.com/stretchr/testify/require"
)

const baselineRequestID = "error-baseline-request"
const baselineSecret = "error-baseline-test-secret"

// These fixtures describe today's wire contract, including its inconsistencies.
// Do not regenerate them from actual responses when changing error handling.
type errorBaselineResponse struct {
	Status      int               `json:"status"`
	ContentType string            `json:"content_type"`
	RetryAfter  string            `json:"retry_after"`
	Headers     map[string]string `json:"headers"`
	JSON        json.RawMessage   `json:"json"`
	Body        string            `json:"body"`
}

func loadErrorBaseline(t *testing.T, path string, count int) map[string]errorBaselineResponse {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixtures map[string]errorBaselineResponse
	require.NoError(t, json.Unmarshal(data, &fixtures))
	require.Len(t, fixtures, count, "each scenario must have an independently reviewed fixture")
	return fixtures
}

func assertErrorBaseline(t *testing.T, fixtures map[string]errorBaselineResponse, name string, response *httptest.ResponseRecorder) {
	t.Helper()
	want, exists := fixtures[name]
	require.True(t, exists, "missing baseline for %s", name)
	require.Equal(t, want.Status, response.Code, response.Body.String())
	require.Equal(t, want.ContentType, response.Header().Get("Content-Type"))
	require.Equal(t, want.RetryAfter, response.Header().Get("Retry-After"))
	for name, value := range want.Headers {
		require.Equal(t, value, response.Header().Get(name), "header %s", name)
	}
	if len(want.JSON) > 0 {
		// JSONEq checks the entire object: missing status, null vs {}, aliases,
		// and unexpected new fields are all observable baseline differences.
		require.JSONEq(t, string(want.JSON), response.Body.String())
	} else {
		require.Equal(t, want.Body, response.Body.String())
	}
}

func baselineRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-Id", baselineRequestID)
	r.RemoteAddr = "127.0.0.1:12345"
	return r
}

func baselineConfig(store *postgres.Storage) router.Config {
	return router.Config{
		Log: slog.New(slog.DiscardHandler), Storage: store,
		AppConfig: &config.Config{Env: "prod", AppSecret: baselineSecret, AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour},
	}
}

type baselineTransport func(*http.Request) (*http.Response, error)

func (f baselineTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The SDK is real, but every provider request stops at this in-memory transport.
func baselineProvider(t *testing.T, cfg *router.Config, body string, failure bool) *int {
	t.Helper()
	calls := new(int)
	transport := baselineTransport(func(r *http.Request) (*http.Response, error) {
		(*calls)++
		if failure {
			return nil, errors.New("synthetic provider unavailable")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	cfg.YooClient = yookassa.NewClient("test-shop", "test-key", opts.WithHTTPClient(http.Client{Transport: transport, Timeout: time.Second}))
	cfg.PaymentHandler = yookassa.NewPaymentHandler(cfg.YooClient)
	return calls
}

func baselineAccount(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM users").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified", "is_premium", "display_name", "auth_version"}).AddRow(7, "user@example.invalid", []byte("unused-test-hash"), false, false, "", 0))
}

// Expected panics use chi's logging hook, without replacing the recoverer.
type baselinePanicLog struct{ calls int }

func (*baselinePanicLog) Write(int, int, http.Header, time.Duration, any) {}
func (l *baselinePanicLog) Panic(any, []byte)                             { l.calls++ }

func TestErrorContractBaseline(t *testing.T) {
	cases := []struct {
		name, method, path, body, authorization string
		repeat                                  int
		setup                                   func(sqlmock.Sqlmock)
		provider                                bool
		providerBody                            string
		providerFailure                         bool
		providerCalls                           int
		inject                                  func(*chi.Mux)
	}{
		{name: "router_not_found", method: "GET", path: "/api/v1/unknown"},
		{name: "router_method_not_allowed", method: "POST", path: "/ping"},
		{name: "dev_route_absent", method: "POST", path: "/api/v1/dev/toggle-premium"},
		{name: "auth_missing", method: "GET", path: "/api/v1/tasks"},
		{name: "auth_header_malformed", method: "GET", path: "/api/v1/tasks", authorization: "Basic invalid"},
		{name: "auth_token_invalid", method: "GET", path: "/api/v1/tasks", authorization: "Bearer invalid"},
		{name: "login_empty", method: "POST", path: "/api/v1/login"},
		{name: "login_malformed", method: "POST", path: "/api/v1/login", body: "{"},
		{name: "login_field_validation", method: "POST", path: "/api/v1/login", body: `{}`},
		{name: "login_body_limit", method: "POST", path: "/api/v1/login", body: `{"extra":"` + strings.Repeat("x", 4096) + `"}`},
		{name: "refresh_body_limit", method: "POST", path: "/api/v1/refresh", body: `{"extra":"` + strings.Repeat("x", 4096) + `"}`},
		{name: "register_malformed", method: "POST", path: "/api/v1/register", body: "{"},
		{name: "register_password_validation", method: "POST", path: "/api/v1/register", body: `{"email":"user@example.invalid","password":"short"}`},
		{name: "forgot_email_validation", method: "POST", path: "/api/v1/password/forgot", body: `{"email":"invalid"}`},
		{name: "login_unknown_account", method: "POST", path: "/api/v1/login", body: `{"email":"user@example.invalid","password":"Password123!"}`, setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("FROM users WHERE email").WithArgs("user@example.invalid").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		}},
		{name: "login_database_error", method: "POST", path: "/api/v1/login", body: `{"email":"user@example.invalid","password":"Password123!"}`, setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("FROM users WHERE email").WithArgs("user@example.invalid").WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "auth_account_unavailable", method: "GET", path: "/api/v1/tasks", authorization: "valid", setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("FROM users").WithArgs(int64(7)).WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "health_database_error", method: "GET", path: "/api/v1/health", setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectPing().WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "account_database_error", method: "POST", path: "/api/v1/register", body: `{"email":"user@example.invalid","password":"Password123!"}`, setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin().WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "problem_database_error", method: "GET", path: "/api/v1/me/focus", authorization: "valid", setup: func(mock sqlmock.Sqlmock) {
			baselineAccount(mock)
			mock.ExpectBegin().WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "operation_database_error", method: "POST", path: "/api/v1/tasks", body: `{"title":"Task"}`, authorization: "valid", setup: func(mock sqlmock.Sqlmock) {
			baselineAccount(mock)
			mock.ExpectBegin().WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "webhook_forbidden", method: "POST", path: "/api/v1/webhooks/yookassa", authorization: "untrusted-peer", body: `{}`},
		{name: "webhook_gateway_unavailable", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{}`},
		{name: "webhook_malformed", method: "POST", path: "/api/v1/webhooks/yookassa", body: "{", provider: true},
		{name: "webhook_ignored_event", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{"event":"payment.waiting_for_capture","object":{"id":"test-payment"}}`, provider: true},
		{name: "webhook_missing_payment_id", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{"event":"payment.succeeded","object":{}}`, provider: true},
		{name: "webhook_provider_error", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{"event":"payment.succeeded","object":{"id":"test-payment"}}`, provider: true, providerFailure: true, providerCalls: 1},
		{name: "webhook_succeeded_database_error", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{"event":"payment.succeeded","object":{"id":"test-payment"}}`, provider: true, providerBody: `{"id":"test-payment","status":"succeeded"}`, providerCalls: 1, setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin().WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "webhook_canceled_database_error", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{"event":"payment.canceled","object":{"id":"test-payment"}}`, provider: true, providerBody: `{"id":"test-payment","status":"canceled"}`, providerCalls: 1, setup: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery("UPDATE payments").WithArgs("canceled", "test-payment").WillReturnError(errors.New("synthetic database error"))
		}},
		{name: "public_rate_limit", method: "GET", path: "/ping", repeat: 101},
		{name: "login_rate_limit", method: "POST", path: "/api/v1/login", body: "{", repeat: 21},
		{name: "invalid_auth_rate_limit", method: "GET", path: "/api/v1/tasks", repeat: 21},
		{name: "webhook_rate_limit", method: "POST", path: "/api/v1/webhooks/yookassa", body: `{}`, repeat: 301},
		{name: "panic_before_write", method: "GET", path: "/__baseline/panic", inject: func(r *chi.Mux) {
			r.Get("/__baseline/panic", func(http.ResponseWriter, *http.Request) { panic("synthetic baseline panic") })
		}},
		{name: "panic_after_write", method: "GET", path: "/__baseline/panic", inject: func(r *chi.Mux) {
			r.Get("/__baseline/panic", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte("partial response"))
				panic("synthetic baseline panic")
			})
		}},
		{name: "webhook_panic_before_write", method: "GET", path: "/__baseline/webhooks/panic", inject: func(r *chi.Mux) {
			r.Get("/__baseline/webhooks/panic", func(http.ResponseWriter, *http.Request) { panic("synthetic webhook baseline panic") })
		}},
		{name: "success_ping", method: "GET", path: "/ping"},
		{name: "success_api_info", method: "GET", path: "/api/v1"},
	}
	fixtures := loadErrorBaseline(t, "testdata/error_contract_baseline.json", len(cases))
	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, seen[tc.name], "duplicate scenario")
			seen[tc.name] = true
			db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			cfg := baselineConfig(&postgres.Storage{DB: db})
			if tc.provider {
				calls := baselineProvider(t, &cfg, tc.providerBody, tc.providerFailure)
				t.Cleanup(func() { require.Equal(t, tc.providerCalls, *calls, "unexpected provider calls") })
			}
			if tc.setup != nil {
				tc.setup(mock)
			}
			handler := router.New(cfg)
			if tc.inject != nil {
				// Test-only fault routes exercise the actual global middleware chain.
				tc.inject(handler)
			}
			for i := 0; i < max(1, tc.repeat); i++ {
				req := baselineRequest(tc.method, tc.path, tc.body)
				panicLog := &baselinePanicLog{}
				if tc.inject != nil {
					req = middleware.WithLogEntry(req, panicLog)
				}
				switch tc.authorization {
				case "valid":
					token, err := jwt.NewAccessToken(domain.User{ID: 7}, baselineSecret, time.Hour)
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+token)
				case "untrusted-peer":
					req.RemoteAddr = "198.51.100.25:12345"
				default:
					req.Header.Set("Authorization", tc.authorization)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if tc.inject != nil {
					require.Equal(t, 1, panicLog.calls)
				}
				if i == max(1, tc.repeat)-1 {
					assertErrorBaseline(t, fixtures, tc.name, response)
				} else {
					require.NotEqual(t, http.StatusTooManyRequests, response.Code, "budget exhausted too early at request %d", i+1)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestErrorContractRequestIDBelongsToCurrentRequest(t *testing.T) {
	handler := router.New(baselineConfig(&postgres.Storage{}))
	var previous string
	for _, supplied := range []string{"first-request", "second-request", "", ""} {
		req := baselineRequest("GET", "/api/v1/unknown", "")
		req.Header.Set("X-Request-Id", supplied)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		require.Equal(t, 404, response.Code)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
		requestID, ok := payload["request_id"].(string)
		require.True(t, ok)
		require.NotEmpty(t, requestID)
		require.NotEqual(t, previous, requestID)
		if supplied != "" {
			require.Equal(t, supplied, requestID)
		}
		previous = requestID
	}
}
