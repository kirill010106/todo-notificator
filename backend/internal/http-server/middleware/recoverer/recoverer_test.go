package recoverer_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/recoverer"
	"github.com/stretchr/testify/require"
)

func TestRecovererResponses(t *testing.T) {
	for _, tc := range []struct {
		name, path, upgrade string
		start               func(http.ResponseWriter)
		status              int
		body                string
		json                bool
	}{
		{name: "before_write", status: 500, json: true},
		{name: "pending_headers", status: 500, json: true, start: func(w http.ResponseWriter) {
			w.Header().Set("Content-Length", "999")
			w.Header().Set("Content-Encoding", "gzip")
		}},
		{name: "upgrade_header_is_not_hijack", upgrade: "Upgrade", status: 500, json: true},
		{name: "after_write", status: 200, body: "partial", start: func(w http.ResponseWriter) { _, _ = w.Write([]byte("partial")) }},
		{name: "after_final_header", status: 204, start: func(w http.ResponseWriter) { w.WriteHeader(204) }},
		{name: "after_flush", status: 200, start: func(w http.ResponseWriter) { require.NoError(t, http.NewResponseController(w).Flush()) }},
		{name: "webhook_before_write", path: "/api/v1/webhooks/yookassa", status: 500},
		{name: "webhook_partial", path: "/api/v1/webhooks/yookassa", status: 200, body: "partial", start: func(w http.ResponseWriter) { _, _ = w.Write([]byte("partial")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer
			handler := middleware.RequestID(recoverer.New(slog.New(slog.NewJSONHandler(&log, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.start != nil {
					tc.start(w)
				}
				panic("private diagnostic")
			})))
			path := tc.path
			if path == "" {
				path = "/api/v1/test"
			}
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("X-Request-Id", "panic-request")
			req.Header.Set("Connection", tc.upgrade)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code)
			if tc.json {
				require.JSONEq(t, `{"code":"INTERNAL_ERROR","message":"Internal Server Error","error":"Internal Server Error","request_id":"panic-request","field_errors":{}}`, response.Body.String())
				require.Equal(t, "application/json", response.Header().Get("Content-Type"))
				require.Empty(t, response.Header().Get("Content-Length"))
				require.Empty(t, response.Header().Get("Content-Encoding"))
			} else {
				require.Equal(t, tc.body, response.Body.String())
			}
			require.NotContains(t, response.Body.String(), "private diagnostic")
			var entry map[string]any
			require.NoError(t, json.Unmarshal(log.Bytes(), &entry)) // exactly one diagnostic
			require.Equal(t, "panic-request", entry["request_id"])
			require.Equal(t, "private diagnostic", entry["panic"])
			require.NotEmpty(t, entry["stack"])
		})
	}
}

func TestAbortHandlerIsNotRecoveredOrLogged(t *testing.T) {
	var log bytes.Buffer
	handler := recoverer.New(slog.New(slog.NewJSONHandler(&log, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	response := httptest.NewRecorder()
	require.PanicsWithValue(t, http.ErrAbortHandler, func() { handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil)) })
	require.Empty(t, response.Body.String())
	require.Empty(t, log.String())
}

type fullWriter struct {
	*httptest.ResponseRecorder
	hijackError error
}

func (w *fullWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, w.hijackError // tests do not use the hijacked connection
}
func (w *fullWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{w.ResponseRecorder}, r)
}
func (w *fullWriter) Push(string, *http.PushOptions) error { return nil }
func (w *fullWriter) CloseNotify() <-chan bool             { return nil }

func TestOptionalWriterInterfacesAndCommitPaths(t *testing.T) {
	for _, action := range []string{"read_from", "hijack", "failed_hijack"} {
		t.Run(action, func(t *testing.T) {
			w := &fullWriter{ResponseRecorder: httptest.NewRecorder()}
			if action == "failed_hijack" {
				w.hijackError = errors.New("hijack failed")
			}
			handler := recoverer.New(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Implements(t, (*http.Flusher)(nil), w)
				require.Implements(t, (*http.Pusher)(nil), w)
				require.Implements(t, (*http.CloseNotifier)(nil), w)
				if action == "read_from" {
					n, err := w.(io.ReaderFrom).ReadFrom(strings.NewReader("copied"))
					require.NoError(t, err)
					require.EqualValues(t, 6, n)
				} else {
					_, _, err := w.(http.Hijacker).Hijack()
					require.Equal(t, action == "failed_hijack", err != nil)
				}
				panic("after optional method")
			}))
			handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			switch action {
			case "read_from":
				require.Equal(t, "copied", w.Body.String())
			case "hijack":
				require.Empty(t, w.Body.String())
			case "failed_hijack":
				require.Equal(t, 500, w.Code)
				require.Contains(t, w.Body.String(), `"code":"INTERNAL_ERROR"`)
			}
		})
	}
	// Unsupported interfaces must remain unsupported, including with a controller.
	plain := struct{ http.ResponseWriter }{httptest.NewRecorder()}
	recoverer.New(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, flush := w.(http.Flusher)
		_, hijack := w.(http.Hijacker)
		require.False(t, flush)
		require.False(t, hijack)
		require.ErrorIs(t, http.NewResponseController(w).Flush(), http.ErrNotSupported)
	})).ServeHTTP(plain, httptest.NewRequest("GET", "/", nil))
}

func TestInformationalHeaderStillAllowsFinalError(t *testing.T) {
	server := httptest.NewServer(middleware.RequestID(recoverer.New(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		panic("after early hints")
	}))))
	defer server.Close()
	seen := 0
	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)
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
	require.Contains(t, string(body), `"code":"INTERNAL_ERROR"`)
}
