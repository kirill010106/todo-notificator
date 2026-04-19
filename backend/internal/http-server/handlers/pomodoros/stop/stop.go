package stop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
	storage.Provider
	StopPomodoroSession(ctx context.Context, userID int64, sessionID int64, finalStatus string) error
	DeletePomodoroSession(ctx context.Context, userID int64, sessionID int64) error
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	UpdateTask(ctx context.Context, userID int64, taskID int64, update domain.TaskUpdate) error
	ApplyStatsDelta(ctx context.Context, userID int64, delta domain.StatsDelta) error
	GetTask(ctx context.Context, userID int64, taskID int64) (domain.Task, error)
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

		user, err := provider.GetUserByID(r.Context(), userID)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				log.Warn("user not found")
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("unauthorized"))
				return
			}
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		log = log.With(slog.Bool("is_verified", user.IsVerified))

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

		if req.Action == domain.PomodoroStatusAbandoned && (session.TaskID == nil || *session.TaskID <= 0) {
			err = provider.DeletePomodoroSession(r.Context(), userID, sessionID)
			if err != nil {
				log.Error("failed to delete abandoned free session", sl.Err(err))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, resp.Error("failed to delete free session"))
				return
			}
			
			log.Info("free pomodoro session abandoned without penalty")
			
			if eventLogger != nil {
				eventLogger.LogEvent(userID, "POMODORO_ABANDONED_FREE", sessionID, map[string]any{})
			}

			render.Status(r, http.StatusOK)
			render.JSON(w, r, resp.OK())
			return
		}

		err = provider.StopPomodoroSession(r.Context(), userID, sessionID, req.Action)
		if err != nil {
			if errors.Is(err, storage.ErrSessionNotFound) {
				log.Warn("session not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("session not found"))
				return
			}
			log.Error("failed to stop pomodoro session", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to stop pomodoro session"))
			return
		}

		statsDelta := domain.StatsDelta{}

		if req.Action == domain.PomodoroStatusCompleted {
			statsDelta.PointsDelta = domain.PomodoroRewardPoints
			statsDelta.PomodorosDelta = 1

			if session.StartedAt != nil && time.Since(*session.StartedAt) < 24*time.Minute {
				log.Info("session duration is too short for rewards", slog.Duration("elapsed", time.Since(*session.StartedAt)))
				statsDelta.PointsDelta = 0
				statsDelta.PomodorosDelta = 0
			}
		} else {
			statsDelta.PointsDelta = domain.PomodoroPenaltyPoints
			if req.FinishTask {
				statsDelta.BurntTasksDelta = 1
				statsDelta.ResetStreak = true
			}
		}

		if session.TaskID != nil && *session.TaskID > 0 {
			updatePayload := domain.TaskUpdate{}
			shouldUpdate := false

			task, err := provider.GetTask(r.Context(), userID, *session.TaskID)
			if err == nil {
				if req.Action == domain.PomodoroStatusCompleted && task.RewardClaimed {
					log.Info("task reward already claimed, resetting points delta for this session")
					statsDelta.PointsDelta = 0
				}
			} else {
				log.Error("failed to get task to check rewards", sl.Err(err))
			}

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

		if eventLogger != nil {
			eventLogger.LogEvent(userID, "POMODORO_STOPPED", sessionID, map[string]any{
				"action": req.Action,
			})
		}

		if statsDelta.PointsDelta == 0 && statsDelta.PomodorosDelta == 0 && statsDelta.BurntTasksDelta == 0 && !statsDelta.ResetStreak {
			log.Info("no stats delta to apply")
			render.Status(r, http.StatusOK)
			render.JSON(w, r, resp.OK())
			return
		}

		err = provider.ApplyStatsDelta(r.Context(), userID, statsDelta)
		if err != nil {
			log.Error("failed to apply stats delta", slog.Any("delta", statsDelta), sl.Err(err))
		} else {
			log.Info("user stats updated successfully", slog.Any("delta", statsDelta))
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, resp.OK())
	}

}
