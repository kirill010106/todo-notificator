package bulk

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type BulkTaskStorage interface {
	BulkCompleteTasks(ctx context.Context, userID int64, taskIDs []int64) (int64, error)
	BulkDeleteTasks(ctx context.Context, userID int64, taskIDs []int64) (int64, error)
}

type Request struct {
	TaskIDs []int64 `json:"task_ids"`
}

type Response struct {
	resp.Response
	AffectedCount int64 `json:"affected_count"`
}

func NewComplete(log *slog.Logger, storage BulkTaskStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.bulk.NewComplete"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request body"))
			return
		}

		if len(req.TaskIDs) == 0 {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("task_ids list cannot be empty"))
			return
		}

		affected, err := storage.BulkCompleteTasks(r.Context(), userID, req.TaskIDs)
		if err != nil {
			log.Error("failed to bulk complete tasks", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to complete tasks"))
			return
		}

		log.Info("tasks bulk completed", slog.Int64("affected", affected))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:      resp.OK(),
			AffectedCount: affected,
		})
	}
}

func NewDelete(log *slog.Logger, storage BulkTaskStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.bulk.NewDelete"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request body"))
			return
		}

		if len(req.TaskIDs) == 0 {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("task_ids list cannot be empty"))
			return
		}

		affected, err := storage.BulkDeleteTasks(r.Context(), userID, req.TaskIDs)
		if err != nil {
			log.Error("failed to bulk delete tasks", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to delete tasks"))
			return
		}

		log.Info("tasks bulk deleted", slog.Int64("affected", affected))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:      resp.OK(),
			AffectedCount: affected,
		})
	}
}
