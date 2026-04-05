package delete

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type TaskDeleter interface {
	DeleteTask(ctx context.Context, userID int64, taskID int64) error
}

func New(log *slog.Logger, taskDeleter TaskDeleter) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.delete.New"

		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		taskIDStr := chi.URLParam(r, "task_id")
		if taskIDStr == "" {
			l.Info("task_id is empty")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("task_id is required"))
			return
		}
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			l.Info("invalid task_id", slog.String("val", taskIDStr))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid task_id format"))
			return
		}

		l = l.With(slog.Int64("task_id", taskID))

		err = taskDeleter.DeleteTask(r.Context(), userID, taskID)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				l.Info("task not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("task not found"))
				return
			}
			l.Error("failed to delete task", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		l.Info("task deleted successfully")

		w.WriteHeader(http.StatusNoContent)
	}
}
