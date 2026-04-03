package stop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/domain"
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
	StopPomodoroSession(ctx context.Context, sessionID int64, finalStatus string) error
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
	UpdateTask(ctx context.Context, userID int64, taskID int64, update domain.TaskUpdate) error
}

var validate = validator.New()

func New(log *slog.Logger, provider PomodoroProvider) http.HandlerFunc {
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
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid session id"))
			return
		}

		log = log.With(slog.Int64("session_id", sessionID))

		var req Request
		err = render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Warn("request body is empty")
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			log.Error("failed to decode request body", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err = validate.Struct(req); err != nil {
			log.Warn("invalid request", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.ValidationError(err.(validator.ValidationErrors)))
			return
		}

		log = log.With(
			slog.String("action", req.Action),
			slog.Bool("finish_task", req.FinishTask),
		)

		session, err := provider.GetActivePomodoroSession(r.Context(), userID)
		if err != nil {
			if errors.Is(err, storage.ErrSessionNotFound) {
				log.Warn("active session not found or belongs to another user")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("active session not found"))
				return
			}
			log.Error("failed to verify active session", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		err = provider.StopPomodoroSession(r.Context(), sessionID, req.Action)
		if err != nil {
			log.Error("failed to stop pomodoro session", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to stop pomodoro session"))
			return
		}

		if session.TaskID != nil && *session.TaskID > 0 {
			updatePayload := domain.TaskUpdate{}
			shouldUpdate := false

			// Increment pomodoros taken if the session successfully completed
			if req.Action == domain.PomodoroStatusCompleted {
				updatePayload.IncrementPomodorosTaken = true
				shouldUpdate = true
			}

			if req.FinishTask {
				var newTaskStatus string
				if req.Action == domain.PomodoroStatusCompleted {
					newTaskStatus = domain.TaskStatusDone
				} else {
					newTaskStatus = domain.TaskStatusBurnt
				}
				updatePayload.Status = &newTaskStatus
				shouldUpdate = true
			}

			if shouldUpdate {
				err = provider.UpdateTask(r.Context(), userID, *session.TaskID, updatePayload)
				if err != nil {
					log.Error("failed to update task", sl.Err(err))
				} else {
					log.Info("task updated by pomodoro action")
				}
			}
		}

		log.Info("pomodoro session stopped successfully")
		render.Status(r, http.StatusOK)
		render.JSON(w, r, resp.OK())
	}

}
