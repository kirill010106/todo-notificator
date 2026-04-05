package get

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

type TaskGetter interface {
	GetTasks(ctx context.Context, userID int64, limit, offset int) ([]domain.Task, int, error)
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
type Response struct {
	resp.Response
	Tasks      []domain.Task `json:"tasks,omitzero"`
	Pagination Pagination    `json:"pagination"`
}

func New(log *slog.Logger, taskGetter TaskGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.tasks.get.New"

		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		limit, offset, err := parsePagination(r)
		if err != nil {
			l.Info("invalid pagination params", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid pagination params"))
			return
		}

		tasks, total, err := taskGetter.GetTasks(r.Context(), userID, limit, offset)

		if err != nil {

			if errors.Is(err, storage.ErrTaskNotFound) {
				l.Info("tasks not found")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("tasks not found"))
				return
			}
			l.Error("failed to get tasks", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get tasks"))
			return
		}

		l.Info("tasks retrieved successfully", slog.Int("task_count", len(tasks)))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Tasks:    tasks,
			Pagination: Pagination{
				Limit:  limit,
				Offset: offset,
				Total:  total,
			},
		})
	}
}

func parsePagination(r *http.Request) (limit, offset int, err error) {
	limit = defaultLimit
	offset = 0

	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v <= 0 {
			return 0, 0, errors.New("invalid limit")
		}
		if v > maxLimit {
			v = maxLimit
		}
		limit = v
	}

	if raw := r.URL.Query().Get("offset"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 0 {
			return 0, 0, errors.New("invalid offset")
		}
		offset = v
	}

	return limit, offset, nil
}
