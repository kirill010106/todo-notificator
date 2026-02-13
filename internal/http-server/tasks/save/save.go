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
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	UserID      int        `json:"user_id" validate:"required,gt=0"`
	Title       string     `json:"title" validate:"required,max=255"`
	Description string     `json:"description" validate:"max=2000"`
	Deadline    *time.Time `json:"deadline"`
}

type Response struct {
	resp.Response
	Id int64 `json:"id,omitempty"`
}

type TaskSaver interface {
	SaveTask(ctx context.Context, t domain.Task) (int64, error)
}

func (r Request) ToDomain() domain.Task {
	return domain.Task{
		UserID:      r.UserID,
		Title:       r.Title,
		Description: r.Description,
		Deadline:    r.Deadline,
		Status:      domain.TaskStatusPending,
	}
}
func New(log *slog.Logger, taskSaver TaskSaver) http.HandlerFunc {
	validate := validator.New()

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.save.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

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

		log.Debug("request body decoded", slog.Any("request", req))

		task := req.ToDomain()

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
		log.Info("task added", slog.Int64("id", id))

		render.Status(r, http.StatusOK)
		responseOK(w, r, id)
	}

}

func responseOK(w http.ResponseWriter, r *http.Request, id int64) {
	render.JSON(w, r, Response{
		Response: resp.OK(),
		Id:       id,
	})
}
