package pause

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type PomodoroProvider interface {
	AddPomodoroBreak(ctx context.Context, userID int64, sessionID int64) error
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

func New(log *slog.Logger, provider PomodoroProvider, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.pomodoros.pause.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		sessionIDStr := chi.URLParam(r, "id")
		sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
		if err != nil || sessionID <= 0 {
			log.Warn("invalid session id", slog.String("session_id", sessionIDStr))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "invalid session id"}, apiresponse.Compatibility{})
			return
		}

		log = log.With(slog.Int64("session_id", sessionID))

		err = provider.AddPomodoroBreak(r.Context(), userID, sessionID)
		if err != nil {
			if errors.Is(err, storage.ErrBreakExhausted) {
				log.Info("user tried to take break but it is exhausted")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusConflict, Code: apiresponse.BreakAlreadyUsed, Message: "extra break is already used"}, apiresponse.Compatibility{})
				return
			}

			if errors.Is(err, storage.ErrSessionNotFound) {
				log.Warn("session not found or not active")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "active session not found"}, apiresponse.Compatibility{})
				return
			}

			log.Error("failed to add pomodoro break", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to add pomodoro break"}, apiresponse.Compatibility{})
			return
		}

		log.Info("pomodoro break added successfully")

		if eventLogger != nil {
			domain.AfterCommit(r.Context(), func() { eventLogger.LogEvent(userID, "POMODORO_PAUSED", sessionID, nil) })
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, resp.OK())
	}
}
