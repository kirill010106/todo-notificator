package logger

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// New returns a structured slog request logger middleware that suppresses noise on health checks and handles log levels cleanly.
func New(log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Don't log high-frequency healthchecks and pings
			if path == "/ping" || path == "/api/v1/health" || path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				duration := time.Since(start)
				status := ww.Status()
				bytes := ww.BytesWritten()

				attrs := []slog.Attr{
					slog.String("request_id", middleware.GetReqID(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", path),
					slog.Int("status", status),
					slog.Duration("duration", duration),
					slog.Int("bytes", bytes),
					slog.String("remote_addr", r.RemoteAddr),
				}

				msg := "http request"

				// Log level routing based on HTTP status
				switch {
				case status >= http.StatusInternalServerError:
					log.LogAttrs(r.Context(), slog.LevelError, msg, attrs...)
				case status == http.StatusUnauthorized || status == http.StatusForbidden:
					// 401/403 are expected client auth scenarios, log as Debug to avoid log pollution
					log.LogAttrs(r.Context(), slog.LevelDebug, msg, attrs...)
				case status >= http.StatusBadRequest:
					log.LogAttrs(r.Context(), slog.LevelWarn, msg, attrs...)
				default:
					// Normal requests
					if strings.HasPrefix(path, "/api/v1") {
						log.LogAttrs(r.Context(), slog.LevelInfo, msg, attrs...)
					} else {
						log.LogAttrs(r.Context(), slog.LevelDebug, msg, attrs...)
					}
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
