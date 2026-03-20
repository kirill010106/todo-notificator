package save

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	Title       string     `json:"title" validate:"required,max=255"`
	Description string     `json:"description" validate:"max=2000"`
	Deadline    *time.Time `json:"deadline,omitzero"`
	ReminderAt  *time.Time `json:"reminder_at,omitzero"`
}

type Response struct {
	resp.Response
	Id int64 `json:"id,omitempty"`
}

type TaskSaver interface {
	SaveTask(ctx context.Context, t domain.Task) (int64, error)
}

func (r Request) ToDomain(userID int64) domain.Task {
	return domain.Task{
		UserID:      userID,
		Title:       r.Title,
		Description: r.Description,
		Deadline:    r.Deadline,
		ReminderAt:  r.ReminderAt,
		Status:      domain.TaskStatusPending,
	}
}

var validate = validator.New()

// TODO: add categories support
func New(log *slog.Logger, taskSaver TaskSaver, webhookURL, webhookSecret string) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.save.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		userID, ok := auth.GetUserID(r.Context())
		if !ok {
			log.Error("user_id not found in context")

			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, resp.Error("unauthorized"))
			return
		}

		log = log.With(slog.Int64("user_id", userID))
		var req Request

		err := render.DecodeJSON(r.Body, &req)
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

		if err := validate.Struct(req); err != nil {
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

		task := req.ToDomain(userID)

		id, err := taskSaver.SaveTask(r.Context(), task)
		if err != nil {
			if errors.Is(err, storage.ErrTaskExists) {
				log.Warn("task already exists", sl.Err(err))

				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("task already exists"))
				return
			}
			log.Error("failed to save task", sl.Err(err))

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to save task"))
			return
		}

		log.Info("task saved", slog.Int64("id", id))

		render.Status(r, http.StatusCreated)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Id:       id,
		})

		if webhookURL != "" {
			go notifyScheduler(log, webhookURL, webhookSecret)
		}
	}
}

func notifyScheduler(log *slog.Logger, url, secret string) {
	req, err := http.NewRequest(http.MethodPost, url+"/webhook/task-created", nil)
	if err != nil {
		log.Warn("webhook: failed to build request", sl.Err(err))
		return
	}
	req.Header.Set("X-Webhook-Secret", secret)

	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Warn("webhook: notifier unavailable, scheduler will catch up via ticker", sl.Err(err))
		return
	}
	defer res.Body.Close()

	log.Debug("webhook: notifier signaled", slog.Int("status", res.StatusCode))
}
