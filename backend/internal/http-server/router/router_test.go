package router_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/stretchr/testify/require"
)

func TestStatsCannotBePatchedInAnyEnvironment(t *testing.T) {
	for _, env := range []string{"local", "prod"} {
		t.Run(env, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			cfg := &config.Config{Env: env, AppSecret: "secret"}
			r := router.New(router.Config{Log: slog.New(slog.DiscardHandler), Storage: &postgres.Storage{DB: db}, AppConfig: cfg})
			token, err := jwt.NewAccessToken(domain.User{ID: 1}, "secret", time.Hour)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/stats", strings.NewReader(`{"points":1000000,"level":100}`))
			req.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			require.Equal(t, http.StatusMethodNotAllowed, response.Code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
