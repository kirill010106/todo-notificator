package contracts

import (
	"encoding/json"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorsNormalizeRateLimitAndKeepLegacyAliases(t *testing.T) {
	handler := middleware.RequestID(Errors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Too many requests", 429)
	})))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/tasks", nil))
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, 429, response.Code)
	require.Equal(t, "RATE_LIMITED", payload["code"])
	require.NotEmpty(t, payload["message"])
	require.NotEmpty(t, payload["request_id"])
	require.Equal(t, "60", response.Header().Get("Retry-After"))
}
