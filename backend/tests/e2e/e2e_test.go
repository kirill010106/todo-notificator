//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/login"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/logout"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/refresh"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/register"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/resend"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/verify"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/create"
	categoriesdelete "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/delete"
	categoriesget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/get"
	categoriesupdate "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/update"
	pomodoropause "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/pause"
	pomodorostart "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/start"
	pomodorostop "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/stop"
	tasksdelete "github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/delete"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/save"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/update"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var (
	suite    *e2eSuite
	suiteErr error
)

const e2ePostgresDSNEnv = "E2E_POSTGRES_DSN"

type e2eSuite struct {
	pg      *tcpostgres.PostgresContainer
	storage *postgres.Storage
	api     *httptest.Server
	webhook *httptest.Server
	http    *http.Client
}

type registerResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	UserID int64  `json:"user_id,omitempty"`
}

type tokensResponse struct {
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

type categoryCreateResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	ID     int64  `json:"id,omitempty"`
}

type taskCreateResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	ID     int64  `json:"id,omitempty"`
}

type taskListResponse struct {
	Status string        `json:"status"`
	Error  string        `json:"error,omitempty"`
	Tasks  []domain.Task `json:"tasks,omitempty"`
}

type pomodoroStartResponse struct {
	Status  string                  `json:"status"`
	Error   string                  `json:"error,omitempty"`
	Session *domain.PomodoroSession `json:"session,omitempty"`
}

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	suite, suiteErr = newE2ESuite(ctx)
	code := m.Run()

	if suite != nil {
		_ = suite.close(context.Background())
	}

	os.Exit(code)
}

func newE2ESuite(ctx context.Context) (*e2eSuite, error) {
	pg, connStr, err := startPostgresForE2E(ctx)
	if err != nil {
		return nil, err
	}

	storage, err := postgres.New(connStr)
	if err != nil {
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("init storage: %w", err)
	}

	goose.SetBaseFS(todonotificator.MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		_ = storage.Close()
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(storage.DB, "migrations"); err != nil {
		_ = storage.Close()
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cfg := &config.Config{
		AppSecret:       "e2e-app-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		Webhook: config.Webhook{
			URL:    webhook.URL,
			Secret: "e2e-webhook-secret",
		},
	}

	log := slog.New(slog.DiscardHandler)
	router := buildRouter(log, storage, cfg)
	api := httptest.NewServer(router)

	return &e2eSuite{
		pg:      pg,
		storage: storage,
		api:     api,
		webhook: webhook,
		http:    api.Client(),
	}, nil
}

func startPostgresForE2E(ctx context.Context) (*tcpostgres.PostgresContainer, string, error) {
	if dsn := strings.TrimSpace(os.Getenv(e2ePostgresDSNEnv)); dsn != "" {
		return nil, dsn, nil
	}

	pg, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("todo_e2e"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		return nil, "", fmt.Errorf("start postgres container (or set %s): %w", e2ePostgresDSNEnv, err)
	}

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pg.Terminate(ctx)
		return nil, "", fmt.Errorf("build postgres connection string: %w", err)
	}

	return pg, connStr, nil
}

func (s *e2eSuite) close(ctx context.Context) error {
	var errs []error

	if s.api != nil {
		s.api.Close()
	}
	if s.webhook != nil {
		s.webhook.Close()
	}
	if s.storage != nil {
		err := s.storage.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if s.pg != nil {
		err := s.pg.Terminate(ctx)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func buildRouter(log *slog.Logger, st *postgres.Storage, cfg *config.Config) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)

	router.Route("/api/v1", func(r chi.Router) {
		r.Post("/register", register.New(log, st, cfg.Webhook.URL, cfg.Webhook.Secret))
		r.Post("/login", login.New(log, st, cfg))
		r.Post("/refresh", refresh.New(log, st, cfg))
		r.Get("/verify", verify.New(log, st))

		r.Group(func(r chi.Router) {
			r.Use(authmw.New(cfg.AppSecret))

			r.Post("/logout", logout.New(log, st))
			r.Get("/tasks", get.New(log, st))
			r.Post("/tasks", save.New(log, st, cfg.Webhook.URL, cfg.Webhook.Secret))
			r.Patch("/tasks/{task_id}", update.New(log, st, cfg.Webhook.URL, cfg.Webhook.Secret))
			r.Delete("/tasks/{task_id}", tasksdelete.New(log, st))

			r.Post("/categories", create.New(log, st))
			r.Get("/categories", categoriesget.New(log, st))
			r.Patch("/categories/{category_id}", categoriesupdate.New(log, st))
			r.Delete("/categories/{category_id}", categoriesdelete.New(log, st))

			r.Post("/pomodoros/start", pomodorostart.New(log, st))
			r.Post("/pomodoros/{id}/pause", pomodoropause.New(log, st))
			r.Post("/pomodoros/{id}/stop", pomodorostop.New(log, st))

			r.Post("/verify/resend", resend.New(log, st, cfg.Webhook.URL, cfg.Webhook.Secret))
		})
	})

	return router
}

func requireSuite(t *testing.T) *e2eSuite {
	t.Helper()
	if suiteErr != nil {
		t.Skipf("e2e environment unavailable: %v", suiteErr)
	}
	require.NotNil(t, suite)
	return suite
}

func randomEmail(prefix string) string {
	safePrefix := strings.ReplaceAll(prefix, "/", "_")
	safePrefix = strings.ReplaceAll(safePrefix, " ", "_")
	return fmt.Sprintf("%s_%d@test.local", safePrefix, time.Now().UnixNano())
}

func (s *e2eSuite) requestJSON(t *testing.T, method string, path string, payload any, bearerToken string) (int, []byte) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, s.api.URL+path, body)
	require.NoError(t, err)

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := s.http.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	respData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, respData
}

func (s *e2eSuite) registerAndLogin(t *testing.T, email string) (int64, string, string) {
	t.Helper()

	regStatus, regBody := s.requestJSON(t, http.MethodPost, "/api/v1/register", map[string]any{
		"email":    email,
		"password": "Password123!",
	}, "")
	require.Equal(t, http.StatusOK, regStatus)

	var regResp registerResponse
	require.NoError(t, json.Unmarshal(regBody, &regResp))
	require.Equal(t, "OK", regResp.Status)
	require.NotZero(t, regResp.UserID)

	loginStatus, loginBody := s.requestJSON(t, http.MethodPost, "/api/v1/login", map[string]any{
		"email":    email,
		"password": "Password123!",
	}, "")
	require.Equal(t, http.StatusOK, loginStatus)

	var tokResp tokensResponse
	require.NoError(t, json.Unmarshal(loginBody, &tokResp))
	require.Equal(t, "OK", tokResp.Status)
	require.NotEmpty(t, tokResp.AccessToken)
	require.NotEmpty(t, tokResp.RefreshToken)

	return regResp.UserID, tokResp.AccessToken, tokResp.RefreshToken
}

func TestE2E_AuthTaskLifecycle(t *testing.T) {
	s := requireSuite(t)
	email := randomEmail("auth_task_lifecycle")
	_, accessToken, refreshToken := s.registerAndLogin(t, email)

	catStatus, catBody := s.requestJSON(t, http.MethodPost, "/api/v1/categories", map[string]any{
		"name": "work",
	}, accessToken)
	require.Equal(t, http.StatusCreated, catStatus)

	var catResp categoryCreateResponse
	require.NoError(t, json.Unmarshal(catBody, &catResp))
	require.Equal(t, "OK", catResp.Status)
	require.NotZero(t, catResp.ID)

	taskStatus, taskBody := s.requestJSON(t, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "e2e task",
		"description": "created in e2e",
		"category_id": catResp.ID,
	}, accessToken)
	require.Equal(t, http.StatusCreated, taskStatus)

	var taskResp taskCreateResponse
	require.NoError(t, json.Unmarshal(taskBody, &taskResp))
	require.Equal(t, "OK", taskResp.Status)
	require.NotZero(t, taskResp.ID)

	listStatus, listBody := s.requestJSON(t, http.MethodGet, "/api/v1/tasks?limit=20&offset=0", nil, accessToken)
	require.Equal(t, http.StatusOK, listStatus)

	var listResp taskListResponse
	require.NoError(t, json.Unmarshal(listBody, &listResp))
	require.Equal(t, "OK", listResp.Status)
	require.NotEmpty(t, listResp.Tasks)

	updateStatus, _ := s.requestJSON(t, http.MethodPatch, "/api/v1/tasks/"+strconv.FormatInt(taskResp.ID, 10), map[string]any{
		"status": "done",
	}, accessToken)
	require.Equal(t, http.StatusOK, updateStatus)

	refreshStatus, refreshBody := s.requestJSON(t, http.MethodPost, "/api/v1/refresh", map[string]any{
		"refresh_token": refreshToken,
	}, "")
	require.Equal(t, http.StatusOK, refreshStatus)

	var rotated tokensResponse
	require.NoError(t, json.Unmarshal(refreshBody, &rotated))
	require.Equal(t, "OK", rotated.Status)
	require.NotEmpty(t, rotated.AccessToken)
	require.NotEmpty(t, rotated.RefreshToken)

	logoutStatus, _ := s.requestJSON(t, http.MethodPost, "/api/v1/logout", map[string]any{
		"refresh_token": rotated.RefreshToken,
	}, rotated.AccessToken)
	require.Equal(t, http.StatusNoContent, logoutStatus)

	secondRefreshStatus, _ := s.requestJSON(t, http.MethodPost, "/api/v1/refresh", map[string]any{
		"refresh_token": rotated.RefreshToken,
	}, "")
	require.Equal(t, http.StatusUnauthorized, secondRefreshStatus)
}

func TestE2E_ProtectedRoutes_RequireAuth(t *testing.T) {
	s := requireSuite(t)

	tasksStatus, _ := s.requestJSON(t, http.MethodGet, "/api/v1/tasks?limit=20&offset=0", nil, "")
	require.Equal(t, http.StatusUnauthorized, tasksStatus)

	categoryStatus, _ := s.requestJSON(t, http.MethodPost, "/api/v1/categories", map[string]any{
		"name": "unauthorized",
	}, "")
	require.Equal(t, http.StatusUnauthorized, categoryStatus)
}

func TestE2E_TaskOwnerIsolation(t *testing.T) {
	s := requireSuite(t)

	ownerEmail := randomEmail("task_owner_a")
	_, ownerToken, _ := s.registerAndLogin(t, ownerEmail)

	attackerEmail := randomEmail("task_owner_b")
	_, attackerToken, _ := s.registerAndLogin(t, attackerEmail)

	taskStatus, taskBody := s.requestJSON(t, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "owner-only task",
		"description": "should not be mutable by other users",
	}, ownerToken)
	require.Equal(t, http.StatusCreated, taskStatus)

	var taskResp taskCreateResponse
	require.NoError(t, json.Unmarshal(taskBody, &taskResp))
	require.NotZero(t, taskResp.ID)

	updateStatus, _ := s.requestJSON(t, http.MethodPatch, "/api/v1/tasks/"+strconv.FormatInt(taskResp.ID, 10), map[string]any{
		"title": "hacked",
	}, attackerToken)
	require.Equal(t, http.StatusNotFound, updateStatus)

	deleteStatus, _ := s.requestJSON(t, http.MethodDelete, "/api/v1/tasks/"+strconv.FormatInt(taskResp.ID, 10), nil, attackerToken)
	require.Equal(t, http.StatusNotFound, deleteStatus)

	ownerListStatus, ownerListBody := s.requestJSON(t, http.MethodGet, "/api/v1/tasks?limit=50&offset=0", nil, ownerToken)
	require.Equal(t, http.StatusOK, ownerListStatus)

	var ownerList taskListResponse
	require.NoError(t, json.Unmarshal(ownerListBody, &ownerList))

	found := false
	for _, task := range ownerList.Tasks {
		if task.ID == taskResp.ID {
			found = true
			require.Equal(t, "owner-only task", task.Title)
			break
		}
	}
	require.True(t, found, "owner task should remain visible to owner")

	ownerDeleteStatus, _ := s.requestJSON(t, http.MethodDelete, "/api/v1/tasks/"+strconv.FormatInt(taskResp.ID, 10), nil, ownerToken)
	require.Equal(t, http.StatusNoContent, ownerDeleteStatus)
}

func TestE2E_RefreshToken_RotationSingleUseRace(t *testing.T) {
	s := requireSuite(t)
	email := randomEmail("refresh_race")
	_, _, oldRefresh := s.registerAndLogin(t, email)

	type refreshAttempt struct {
		status       int
		refreshToken string
		err          error
	}

	const concurrentRequests = 2
	results := make(chan refreshAttempt, concurrentRequests)

	var wg sync.WaitGroup
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			status, body := s.requestJSON(t, http.MethodPost, "/api/v1/refresh", map[string]any{
				"refresh_token": oldRefresh,
			}, "")

			if status == http.StatusOK {
				var resp tokensResponse
				err := json.Unmarshal(body, &resp)
				if err != nil {
					results <- refreshAttempt{status: status, err: fmt.Errorf("decode refresh response: %w", err)}
					return
				}
				results <- refreshAttempt{status: status, refreshToken: resp.RefreshToken}
				return
			}

			results <- refreshAttempt{status: status}
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	unauthorizedCount := 0
	var validRotatedToken string
	for result := range results {
		require.NoError(t, result.err)

		switch result.status {
		case http.StatusOK:
			successCount++
			if result.refreshToken != "" {
				validRotatedToken = result.refreshToken
			}
		case http.StatusUnauthorized:
			unauthorizedCount++
		}
	}

	require.Equal(t, 1, successCount, "expected exactly one successful rotation")
	require.Equal(t, 1, unauthorizedCount, "expected exactly one rejected reused token")
	require.NotEmpty(t, validRotatedToken)

	status, _ := s.requestJSON(t, http.MethodPost, "/api/v1/refresh", map[string]any{
		"refresh_token": validRotatedToken,
	}, "")
	require.Equal(t, http.StatusOK, status)
}

func TestE2E_Pomodoro_OwnerIsolation(t *testing.T) {
	s := requireSuite(t)

	emailA := randomEmail("pomodoro_owner_a")
	_, tokenA, _ := s.registerAndLogin(t, emailA)

	emailB := randomEmail("pomodoro_owner_b")
	_, tokenB, _ := s.registerAndLogin(t, emailB)

	startAStatus, startABody := s.requestJSON(t, http.MethodPost, "/api/v1/pomodoros/start", map[string]any{}, tokenA)
	require.Equal(t, http.StatusCreated, startAStatus)

	var startAResp pomodoroStartResponse
	require.NoError(t, json.Unmarshal(startABody, &startAResp))
	require.NotNil(t, startAResp.Session)
	sessionAID := startAResp.Session.ID
	require.NotZero(t, sessionAID)

	startBStatus, _ := s.requestJSON(t, http.MethodPost, "/api/v1/pomodoros/start", map[string]any{}, tokenB)
	require.Equal(t, http.StatusCreated, startBStatus)

	attackStatus, _ := s.requestJSON(t, http.MethodPost, "/api/v1/pomodoros/"+strconv.FormatInt(sessionAID, 10)+"/stop", map[string]any{
		"action": "abandoned",
	}, tokenB)
	require.Equal(t, http.StatusNotFound, attackStatus)

	ownerStopStatus, _ := s.requestJSON(t, http.MethodPost, "/api/v1/pomodoros/"+strconv.FormatInt(sessionAID, 10)+"/stop", map[string]any{
		"action": "abandoned",
	}, tokenA)
	require.Equal(t, http.StatusOK, ownerStopStatus)
}
