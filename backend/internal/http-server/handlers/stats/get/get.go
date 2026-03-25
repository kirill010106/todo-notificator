package get

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type UserStatsGetter interface {
	GetUserStats(ctx context.Context, userID int64) (domain.UserStats, error)
}

type Response struct {
	resp.Response
	UserStats domain.UserStats `json:"user_stats"`
}

func New(log *slog.Logger, userStatsGetter UserStatsGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.stats.get.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		stats, err := userStatsGetter.GetUserStats(r.Context(), userID)
		if err != nil {
			log.Error("failed to get user stats", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get user stats"))
			return
		}

		log.Info("user stats retrieved successfully")

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:  resp.OK(),
			UserStats: stats,
		})
	}
}
