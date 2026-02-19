package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/backend/internal/domain"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/middleware/auth"
	resp "github.com/kirill010106/todo-notificator/backend/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/backend/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/backend/internal/storage"
)

type Request struct {
	Title       *string    `json:"title" validate:"omitempty,max=255"`
	Description *string    `json:"description" validate:"omitempty,max=2000"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Status      *string    `json:"status,omitempty" validate:"omitempty,oneof=pending done"`
}

type Response struct {
	resp.Response
	TaskID int64 `json:"task_id"`
}

type TaskUpdater interface {
	UpdateTask(ctx context.Context, userID int64, taskID int64, task domain.TaskUpdate) error
}

var validate = validator.New()

func formatValidationError(err validator.FieldError) string {
	field := strings.ToLower(err.Field())

	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "max":
		return fmt.Sprintf("%s must not exceed %s characters", field, err.Param())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", field, err.Param())
	case "email":
		return fmt.Sprintf("%s must be a valid email", field)
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, err.Param())
	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

func (r Request) ToDomain() domain.TaskUpdate {
	return domain.TaskUpdate{
		Title:       r.Title,
		Description: r.Description,
		Deadline:    r.Deadline,
		Status:      r.Status,
	}
}

func (r Request) IsEmpty() bool {
	return r.Title == nil &&
		r.Description == nil &&
		r.Deadline == nil &&
		r.Status == nil
}

func New(log *slog.Logger, taskUpdater TaskUpdater) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.update.New"

		log = log.With(
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

		if err := validate.Struct(req); err != nil {
			var validateErr validator.ValidationErrors

			if errors.As(err, &validateErr) {
				log.Warn("invalid request", sl.Err(err))

				errMsgs := make([]string, 0, len(validateErr))
				for _, e := range validateErr {
					errMsgs = append(errMsgs, formatValidationError(e))
				}
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("validation failed: "+strings.Join(errMsgs, "; ")))
				return
			}

			log.Error("internal validation error", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal server error"))
			return
		}

		log.Debug("request body decoded", slog.Any("request", req))

		taskUpdate := req.ToDomain()
		err = taskUpdater.UpdateTask(r.Context(), userID, taskID, taskUpdate)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				log.Info("task not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("task not found"))
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
	}
}
