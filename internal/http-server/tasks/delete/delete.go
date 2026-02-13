package delete

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type TaskDeleter interface {
	DeleteTask(ctx context.Context, userID int64, taskId int64) (int64, error)
}
type Request struct {
	UserID int64 `json:"user_id" validate:"required,gt=0"`
	TaskID int64 `json:"task_id" validate:"required"`
}

type Response struct {
	resp.Response
	TaskID int64 `json:"id"`
}

func New(log *slog.Logger, taskDeleter TaskDeleter) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.delete.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		userIDStr := chi.URLParam(r, "user_id")
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			log.Info("invalid user_id", slog.String("val", userIDStr))
			RenderBadRequest(r)
			render.JSON(w, r, resp.Error("invalid user_id format"))
			return
		}
		taskIDStr := chi.URLParam(r, "task_id")
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			log.Info("invalid task_id", slog.String("val", userIDStr))
			RenderBadRequest(r)
			render.JSON(w, r, resp.Error("invalid task_id format"))
			return
		}

		id, err := taskDeleter.DeleteTask(r.Context(), userID, taskID)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				log.Info("task not found", slog.Int64("task_id", taskID))
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("task not found"))
				return
			}

			log.Error("failed to delete task", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		log.Info("task deleted", slog.Int64("id", id))

		render.JSON(w, r, Response{
			Response: resp.OK(),
			TaskID:   id,
		})
	}
}

func RenderBadRequest(r *http.Request) {
	render.Status(r, http.StatusBadRequest)
}
