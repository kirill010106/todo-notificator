package stop

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	Action     string `json:"action" validate:"required,oneof=abandoned completed"`
	FinishTask bool   `json:"finish_task,omitempty"`
}

type PomodoroProvider interface {
	FinishPomodoroSession(ctx context.Context, userID int64, sessionID int64, action string, finishTask bool) error
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

var validate = validator.New()

func New(log *slog.Logger, provider PomodoroProvider, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.pomodoros.stop.New"

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

		var req Request
		err = render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Warn("request body is empty")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "empty request body"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to decode request body", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "failed to decode request"}, apiresponse.Compatibility{})
			return
		}

		if err = validate.Struct(req); err != nil {
			var validationErrors validator.ValidationErrors
			if errors.As(err, &validationErrors) {
				helpers.RenderValidationError(w, r, validationErrors)
			} else {
				log.Error("internal validation error", sl.Err(err))
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal server error"}, apiresponse.Compatibility{})
			}
			return
		}

		err = provider.FinishPomodoroSession(r.Context(), userID, sessionID, req.Action, req.FinishTask)
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrUserNotFound):
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "unauthorized"}, apiresponse.Compatibility{})
			case errors.Is(err, storage.ErrSessionNotFound), errors.Is(err, storage.ErrTaskNotFound):
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "active session or task not found"}, apiresponse.Compatibility{})
			default:
				log.Error("failed to finish pomodoro transaction", sl.Err(err))
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to stop pomodoro session"}, apiresponse.Compatibility{})
			}
			return
		}
		if eventLogger != nil {
			domain.AfterCommit(r.Context(), func() {
				eventLogger.LogEvent(userID, "POMODORO_STOPPED", sessionID, map[string]any{"action": req.Action})
			})
		}
		render.Status(r, http.StatusOK)
		render.JSON(w, r, resp.OK())
	}
}
