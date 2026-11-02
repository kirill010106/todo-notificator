package recoverer

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"

	"github.com/felixge/httpsnoop"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
)

// New recovers application panics while preserving an already committed or
// hijacked response. It must run inside request logging and metrics middleware.
func New(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started, hijacked := false, false
			tracked := httpsnoop.Wrap(w, httpsnoop.Hooks{
				WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
					return func(status int) {
						next(status)
						if status >= 200 || status == http.StatusSwitchingProtocols {
							started = true
						}
					}
				},
				Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
					return func(body []byte) (int, error) {
						started = true
						return next(body)
					}
				},
				Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
					return func() {
						if !started {
							w.WriteHeader(http.StatusOK)
						}
						started = true
						next()
					}
				},
				ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
					return func(src io.Reader) (int64, error) {
						if !started {
							w.WriteHeader(http.StatusOK)
						}
						started = true
						return next(src)
					}
				},
				Hijack: func(next httpsnoop.HijackFunc) httpsnoop.HijackFunc {
					return func() (net.Conn, *bufio.ReadWriter, error) {
						conn, rw, err := next()
						if err == nil {
							hijacked = true
						}
						return conn, rw, err
					}
				},
			})
			defer func() {
				if cause := recover(); cause != nil {
					if cause == http.ErrAbortHandler {
						panic(cause)
					}
					if entry := middleware.GetLogEntry(r); entry != nil {
						entry.Panic(cause, debug.Stack())
					} else {
						log.ErrorContext(r.Context(), "http panic",
							"request_id", middleware.GetReqID(r.Context()),
							"panic", cause, "stack", string(debug.Stack()))
					}
					if started || hijacked {
						return
					}
					w.Header().Del("Content-Length")
					w.Header().Del("Content-Encoding")
					if apiresponse.IsWebhook(r) {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					apiresponse.Write(w, r, apiresponse.Problem{
						Status: http.StatusInternalServerError, Code: apiresponse.InternalError,
						Message: http.StatusText(http.StatusInternalServerError),
					}, apiresponse.Compatibility{StatusStyle: apiresponse.OmitStatus})
				}
			}()
			next.ServeHTTP(tracked, r)
		})
	}
}
