package delete

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type TaskDeleter interface {
	DeleteTask(ctx context.Context, userID int64, taskID int64) error
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

func New(log *slog.Logger, taskDeleter TaskDeleter, eventLogger EventLogger) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.delete.New"

		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		taskIDStr := chi.URLParam(r, "task_id")
		if taskIDStr == "" {
			l.Info("task_id is empty")
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "task_id is required"}, apiresponse.Compatibility{})
			return
		}
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			l.Info("invalid task_id", slog.String("val", taskIDStr))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "invalid task_id format"}, apiresponse.Compatibility{})
			return
		}

		l = l.With(slog.Int64("task_id", taskID))

		err = taskDeleter.DeleteTask(r.Context(), userID, taskID)
		if err != nil {
			if errors.Is(err, storage.ErrTaskNotFound) {
				l.Info("task not found or access denied")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "task not found"}, apiresponse.Compatibility{})
				return
			}
			l.Error("failed to delete task", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal error"}, apiresponse.Compatibility{})
			return
		}

		l.Info("task deleted successfully")

		if eventLogger != nil {
			eventLogger.LogEvent(userID, "TASK_DELETED", taskID, nil)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
