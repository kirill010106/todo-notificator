package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func requestCount(t *testing.T, path, status string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["method"] == "GET" && labels["path"] == path && labels["status"] == status {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestProductionRouterDoesNotBufferFlushedResponse(t *testing.T) {
	unblock := make(chan struct{})
	release := sync.OnceFunc(func() { close(unblock) })
	handler := router.New(baselineConfig(&postgres.Storage{}))
	handler.Get("/__infrastructure/stream", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			panic(err)
		}
		select {
		case <-unblock:
			_, _ = w.Write([]byte("second"))
		case <-r.Context().Done():
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/__infrastructure/stream", nil)
	require.NoError(t, err)
	response, err := server.Client().Do(req)
	require.NoError(t, err, "Flush must deliver headers while the handler waits for the client")
	defer response.Body.Close()
	first := make([]byte, len("first"))
	_, err = io.ReadFull(response.Body, first)
	require.NoError(t, err)
	require.Equal(t, "first", string(first))
	release() // Only after receiving the prefix may the handler finish.
	rest, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "second", string(rest))
	require.Equal(t, 200, response.StatusCode)
}

func TestProductionRouterAllowsFinalErrorAfterEarlyHints(t *testing.T) {
	handler := router.New(baselineConfig(&postgres.Storage{}))
	handler.Get("/__infrastructure/hints", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		panic("private diagnostic")
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/__infrastructure/hints", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-Id", baselineRequestID)
	seen := 0
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
		require.Equal(t, 103, code)
		seen++
		return nil
	}}))
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 1, seen)
	require.Equal(t, 500, response.StatusCode)
	require.JSONEq(t, `{"code":"INTERNAL_ERROR","message":"Internal Server Error","error":"Internal Server Error","request_id":"error-baseline-request","field_errors":{}}`, string(body))
}

func TestInfrastructurePanicStatusReachesLogsAndMetrics(t *testing.T) {
	for _, mode := range []string{"before", "partial", "flush"} {
		t.Run(mode, func(t *testing.T) {
			var log bytes.Buffer
			cfg := baselineConfig(&postgres.Storage{})
			cfg.Log = slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
			handler := router.New(cfg)
			path := "/__infrastructure/" + mode
			handler.Get(path, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "partial":
					_, _ = w.Write([]byte("partial"))
				case "flush":
					require.NoError(t, http.NewResponseController(w).Flush())
				}
				panic("private cause")
			})
			status, label := 200, "200"
			if mode == "before" {
				status, label = 500, "500"
			}
			before := requestCount(t, path, label)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, baselineRequest("GET", path, ""))
			require.Equal(t, status, response.Code)
			require.Equal(t, before+1, requestCount(t, path, label))
			if mode == "partial" {
				require.Equal(t, "partial", response.Body.String())
			} else if mode == "flush" {
				require.Empty(t, response.Body.String())
			}
			requests, diagnostics := 0, 0
			for _, line := range strings.Split(strings.TrimSpace(log.String()), "\n") {
				var entry map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &entry))
				require.Equal(t, baselineRequestID, entry["request_id"])
				if entry["msg"] == "http request" {
					requests++
					require.EqualValues(t, status, entry["status"])
				} else if entry["msg"] == "http panic" {
					diagnostics++
				}
			}
			require.Equal(t, 1, requests)
			require.Equal(t, 1, diagnostics)
		})
	}
}
