package start

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	TaskID *int64 `json:"task_id,omitzero"`
}

type Response struct {
	resp.Response
	Session *domain.PomodoroSession `json:"session,omitempty"`
}

type ActiveSessionResponse struct {
	resp.Response
	ActiveSession *domain.PomodoroSession `json:"active_session,omitempty"`
}

type PomodoroProvider interface {
	StartPomodoroSession(ctx context.Context, userID int64, taskID *int64) (*domain.PomodoroSession, error)
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

func New(log *slog.Logger, provider PomodoroProvider, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.pomodoros.start.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Warn("failed to decode request body", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		session, err := provider.StartPomodoroSession(r.Context(), userID, req.TaskID)
		if err != nil {
			if errors.Is(err, storage.ErrSessionActive) {
				log.Info("user already has an active pomodoro session")

				activeSession, getErr := provider.GetActivePomodoroSession(r.Context(), userID)
				if getErr != nil {
					log.Error("failed to get active pomodoro session details", sl.Err(getErr))
					render.Status(r, http.StatusConflict)
					render.JSON(w, r, resp.Error("pomodoro session is already active"))
					return
				}

				render.Status(r, http.StatusConflict)
				render.JSON(w, r, ActiveSessionResponse{
					Response:      resp.Error("pomodoro session is already active"),
					ActiveSession: activeSession,
				})
				return
			}

			log.Error("failed to start pomodoro session", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to start pomodoro session"))
			return
		}

		log.Info("pomodoro session started successfully", slog.Int64("session_id", session.ID))

		if eventLogger != nil {
			details := map[string]any{}
			if req.TaskID != nil {
				details["task_id"] = *req.TaskID
			}
			eventLogger.LogEvent(userID, "POMODORO_STARTED", session.ID, details)
		}

		render.Status(r, http.StatusCreated)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Session:  session,
		})
	}
}
