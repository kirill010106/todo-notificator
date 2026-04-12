package helpers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
)

// LoggerWithAuth returns constructed logger with op, request_id and userID
// If unauthorized - writes to response and returns nil, false
func LoggerWithAuth(w http.ResponseWriter, r *http.Request, log *slog.Logger, op string) (*slog.Logger, int64, bool) {
	l := log.With(
		slog.String("op", op),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		l.Error("user_id not found in context")
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, resp.Error("unauthorized"))
		return nil, 0, false
	}

	l = l.With(slog.Int64("user_id", userID))
	return l, userID, true
}
