package router

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/stretchr/testify/require"
)

func TestPublicAuthRoutesRejectLargeBodyBeforeDatabaseAccess(t *testing.T) {
	router := New(Config{Log: slog.New(slog.DiscardHandler), Storage: &postgres.Storage{}, AppConfig: &config.Config{AppSecret: "test"}})
	for _, path := range []string{"/login", "/refresh"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1"+path, strings.NewReader(`{"unknown":"`+strings.Repeat("x", 8<<20)+`"}`)))
		require.Equal(t, 413, w.Code)
		require.Contains(t, w.Body.String(), "PAYLOAD_TOO_LARGE")
	}
}
