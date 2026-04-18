package update

import (
	"bytes"
	"context"
	"encoding/json"
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
	Title       *string    `json:"title" validate:"omitempty,max=255"`
	Description *string    `json:"description" validate:"omitempty,max=2000"`
	Deadline    *time.Time `json:"deadline,omitzero"`
	ReminderAt  *time.Time `json:"reminder_at,omitzero"`
	Status      *string    `json:"status,omitempty" validate:"omitempty,oneof=pending done burnt"`
	CategoryID  *int64     `json:"category_id,omitempty" validate:"omitempty,gt=0"`
}

type Response struct {
	resp.Response
	TaskID int64 `json:"task_id"`
}

type TaskUpdater interface {
	GetTask(ctx context.Context, userID int64, taskID int64) (domain.Task, error)
	UpdateTask(ctx context.Context, userID int64, taskID int64, task domain.TaskUpdate) error
	ApplyStatsDelta(ctx context.Context, userID int64, delta domain.StatsDelta) error
}

var validate = validator.New()

type schedulerWebhookPayload struct {
	Type string `json:"type"`
}

func (r Request) ToDomain() domain.TaskUpdate {
	return domain.TaskUpdate{
		Title:       r.Title,
		Description: r.Description,
		Deadline:    r.Deadline,
		ReminderAt:  r.ReminderAt,
		Status:      r.Status,
		CategoryID:  r.CategoryID,
	}
}

func (r Request) IsEmpty() bool {
	return r.Title == nil &&
		r.Description == nil &&
		r.Deadline == nil &&
		r.Status == nil &&
		r.ReminderAt == nil &&
		r.CategoryID == nil
}

func New(log *slog.Logger, taskUpdater TaskUpdater, webhookURL, webhookSecret string) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.update.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		taskIDStr := chi.URLParam(r, "task_id")
		if taskIDStr == "" {
			log.Warn("task_id is empty")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("task_id is required"))
			return
		}

		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			log.Warn("invalid task id", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid task_id"))
			return
		}

		log = log.With(slog.Int64("task_id", taskID))

		var req Request

		err = render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Warn("request body is empty")

				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			log.Warn("failed to decode request body", sl.Err(err))

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if req.IsEmpty() {
			log.Warn("no fields to update")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("at least one field must be provided"))
			return
		}

		if err = validate.Struct(req); err != nil {
			var validateErr validator.ValidationErrors

			if errors.As(err, &validateErr) {
				log.Warn("invalid request", sl.Err(err))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.ValidationError(validateErr))
				return
			}

			log.Error("internal validation error", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal server error"))
			return
		}

		if req.ReminderAt != nil && req.ReminderAt.Before(time.Now()) {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("reminder_at must be in the future"))
			return
		}

		log.Debug("request body decoded", slog.Any("request", req))

		taskUpdate := req.ToDomain()
		existingTask, err := taskUpdater.GetTask(r.Context(), userID, taskID)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				log.Info("task not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("task not found"))
				return
			}
			log.Error("failed to get existing task", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get task"))
			return
		}
		if req.Status != nil && *req.Status == domain.TaskStatusDone && existingTask.Status != domain.TaskStatusDone {
			if !existingTask.RewardClaimed {
				statsDelta := domain.StatsDelta{
					PointsDelta: domain.TaskCompleteReward,
				}

				err := taskUpdater.ApplyStatsDelta(r.Context(), userID, statsDelta)
				if err != nil {
					log.Error("failed to apply return reward", sl.Err(err))
				} else {
					log.Info("reward granted for task completion", slog.Int("points", domain.TaskCompleteReward))
				}

				taskUpdate.RewardClaimed = true
			}
		}
		err = taskUpdater.UpdateTask(r.Context(), userID, taskID, taskUpdate)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				log.Info("task not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("task not found"))
				return
			}
			if errors.Is(err, storage.ErrCategoryNotFound) {
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("category not found"))
				return
			}
			log.Error("failed to update task", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to update task"))
			return
		}

		log.Info("task updated successfully")

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			TaskID:   taskID,
		})

		if webhookURL != "" {
			go notifyScheduler(log, webhookURL, webhookSecret, "task_updated")
		}
	}
}

func notifyScheduler(log *slog.Logger, url, secret, eventType string) {
	body, err := json.Marshal(schedulerWebhookPayload{Type: eventType})
	if err != nil {
		log.Warn("webhook: failed to marshal payload", sl.Err(err))
		return
	}

	req, err := http.NewRequest(http.MethodPost, url+"/webhook/task-created", bytes.NewReader(body))
	if err != nil {
		log.Warn("webhook: failed to build request", sl.Err(err))
		return
	}
	req.Header.Set("X-Webhook-Secret", secret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Warn("webhook: notifier unavailable, scheduler will catch up via ticker", sl.Err(err))
		return
	}
	defer res.Body.Close()

	log.Debug("webhook: notifier signaled", slog.Int("status", res.StatusCode), slog.String("type", eventType))
}
