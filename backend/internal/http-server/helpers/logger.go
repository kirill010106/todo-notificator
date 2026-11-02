package helpers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
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
		apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "unauthorized"}, apiresponse.Compatibility{})
		return nil, 0, false
	}
	isPremium, ok := auth.GetPremiumStatus(r.Context())
	if !ok {
		l.Warn("premium status not found in context")
		apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal error"}, apiresponse.Compatibility{})
		return nil, 0, false
	}
	l = l.With(slog.Int64("user_id", userID), slog.Bool("is_premium", isPremium))
	return l, userID, true
}
