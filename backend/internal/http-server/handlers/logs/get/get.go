package get

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type LogsGetter interface {
	GetLogs(ctx context.Context, userID int64, limit, offset int32) ([]domain.ActivityLog, error)
}

type Response struct {
	resp.Response
	Logs []domain.ActivityLog `json:"logs"`
}

func New(log *slog.Logger, logsGetter LogsGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.logs.get.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		limitRaw := r.URL.Query().Get("limit")
		limit := int32(50) // Default limit
		if limitRaw != "" {
			if parsed, err := strconv.ParseInt(limitRaw, 10, 32); err == nil && parsed > 0 {
				limit = int32(parsed)
			}
		}

		offsetRaw := r.URL.Query().Get("offset")
		offset := int32(0) // Default offset
		if offsetRaw != "" {
			if parsed, err := strconv.ParseInt(offsetRaw, 10, 32); err == nil && parsed >= 0 {
				offset = int32(parsed)
			}
		}

		logs, err := logsGetter.GetLogs(r.Context(), userID, limit, offset)
		if err != nil {
			log.Error("failed to get logs", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get logs"))
			return
		}

		if logs == nil {
			logs = []domain.ActivityLog{}
		}

		log.Info("logs fetched successfully", slog.Int("count", len(logs)))

		render.JSON(w, r, Response{
			Response: resp.OK(),
			Logs:     logs,
		})
	}
}
