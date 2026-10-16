package save

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"

	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	Title       string                 `json:"title" validate:"required,max=255"`
	Description string                 `json:"description" validate:"max=2000"`
	Deadline    *time.Time             `json:"deadline,omitzero"`
	Reminders   []domain.ReminderInput `json:"reminders"`
	CategoryID  *int64                 `json:"category_id,omitzero" validate:"omitzero,gt=0"`
}

func (r *Request) UnmarshalJSON(data []byte) error {
	if err := domain.ValidateReminderFields(data); err != nil {
		return err
	}
	type plain Request
	return json.Unmarshal(data, (*plain)(r))
}

type Response struct {
	resp.Response
	ID int64 `json:"id,omitempty"`
}

type TaskSaver interface {
	SaveTask(ctx context.Context, t domain.Task) (int64, error)
	CountActiveReminders(ctx context.Context, userID int64) (int, error)
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

func (r Request) ToDomain(userID int64) domain.Task {
	task := domain.Task{
		UserID:         userID,
		Title:          r.Title,
		Description:    r.Description,
		Deadline:       r.Deadline,
		ReminderInputs: r.Reminders,
		CategoryID:     r.CategoryID,
		Status:         domain.TaskStatusPending,
	}
	return task
}

var validate = validator.New()

type schedulerWebhookPayload struct {
	Type string `json:"type"`
}

func New(log *slog.Logger, taskSaver TaskSaver, webhookURL, webhookSecret string, eventLogger EventLogger) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.save.New"

		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		var req Request

		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if helpers.RenderContractError(w, r, err) {
				return
			}
			if errors.Is(err, io.EOF) {
				l.Warn("request body is empty")

				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			l.Warn("failed to decode request body", sl.Err(err))

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("Некорректный запрос: "+err.Error()))
			return
		}

		if err = validate.Struct(req); err != nil {
			var validateErr validator.ValidationErrors

			if errors.As(err, &validateErr) {
				l.Warn("invalid request", sl.Err(err))

				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.ValidationError(validateErr))
				return
			}
			l.Error("internal validation error", sl.Err(err))

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal server error"))
			return
		}

		l.Debug("request body decoded", slog.Any("request", req))

		task := req.ToDomain(userID)

		id, err := taskSaver.SaveTask(r.Context(), task)
		if err != nil {
			if helpers.RenderContractError(w, r, err) {
				return
			}
			if errors.Is(err, storage.ErrTaskExists) {
				l.Warn("task already exists", sl.Err(err))

				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("task already exists"))
				return
			}
			if errors.Is(err, storage.ErrCategoryNotFound) {
				l.Warn("category not found", sl.Err(err))

				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("category not found"))
				return
			}
			l.Error("failed to save task", sl.Err(err))

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to save task"))
			return
		}

		l.Info("task saved", slog.Int64("id", id))

		render.Status(r, http.StatusCreated)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			ID:       id,
		})

		if eventLogger != nil {
			domain.AfterCommit(r.Context(), func() {
				eventLogger.LogEvent(userID, "TASK_CREATED", id, map[string]any{
					"title":       req.Title,
					"category_id": req.CategoryID,
				})
			})
		}

		if webhookURL != "" {
			domain.AfterCommit(r.Context(), func() { go notifyScheduler(l, webhookURL, webhookSecret, "task_created") })
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
