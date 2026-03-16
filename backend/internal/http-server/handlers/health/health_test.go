package health

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockDB struct {
	err error
}

func (m mockDB) PingContext(context.Context) error {
	return m.err
}

func TestHealth_OK(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockDB{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"OK"`)
}

func TestHealth_DBUnavailable(t *testing.T) {
	h := New(slog.New(slog.DiscardHandler), mockDB{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "database is unreachable")
}
