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
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type TaskDeleter interface {
	DeleteTask(ctx context.Context, userID int64, taskId int64) error
}

func New(log *slog.Logger, taskDeleter TaskDeleter) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.delete.New"

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

		taskIDStr := chi.URLParam(r, "task_id")
		if taskIDStr == "" {
			log.Info("task_id is empty")
			RenderBadRequest(r)
			render.JSON(w, r, resp.Error("task_id is required"))
			return
		}
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			log.Info("invalid task_id", slog.String("val", taskIDStr))
			RenderBadRequest(r)
			render.JSON(w, r, resp.Error("invalid task_id format"))
			return
		}

		log = log.With(slog.Int64("task_id", taskID))

		err = taskDeleter.DeleteTask(r.Context(), userID, taskID)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				log.Info("task not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("task not found"))
				return
			}
			log.Error("failed to delete task", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		log.Info("task deleted successfully")

		w.WriteHeader(http.StatusNoContent)
	}
}

func RenderBadRequest(r *http.Request) {
	render.Status(r, http.StatusBadRequest)
}
