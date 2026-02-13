package get

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type TaskGetter interface {
	GetTasks(ctx context.Context, userID int64) ([]domain.Task, error)
}
type Response struct {
	resp.Response
	Tasks []domain.Task `json:"tasks,omitempty"`
}

func New(log *slog.Logger, taskGetter TaskGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.get.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		userIDStr := chi.URLParam(r, "user_id")
		if userIDStr == "" {
			log.Info("user_id parameter is missing")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request"))
			return
		}

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			log.Info("invalid user_id format", slog.String("user_id", userIDStr))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid user_id format"))
			return
		}

		tasks, err := taskGetter.GetTasks(r.Context(), userID)
		if err != nil {
			log.Error("failed to get tasks", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get tasks"))
			return
		}

		log.Info("tasks retrieved successfully", slog.Int("task_count", len(tasks)))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Tasks:    tasks,
		})
	}
}
