package update

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type Request struct {
	Points          *int64     `json:"points"`
	Level           *int64     `json:"level"`
	TotalPomodoros  *int64     `json:"total_pomodoros,omitzero"`
	TotalBurntTasks *int64     `json:"total_burnt_tasks,omitzero"`
	CurrentStreak   *int64     `json:"current_streak,omitzero"`
	BestStreak      *int64     `json:"best_streak,omitzero"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

type Response struct {
	resp.Response
	UserID int64 `json:"user_id"`
}

type UserStatsUpdater interface {
	UpdateUserStats(ctx context.Context, userID int64, stats domain.UserStatsUpdate) error
}

func (r Request) ToDomain() domain.UserStatsUpdate {
	return domain.UserStatsUpdate{
		Points:          r.Points,
		Level:           r.Level,
		TotalPomodoros:  r.TotalPomodoros,
		TotalBurntTasks: r.TotalBurntTasks,
		CurrentStreak:   r.CurrentStreak,
		BestStreak:      r.BestStreak,
	}
}

func New(log *slog.Logger, userStatsUpdater UserStatsUpdater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.stats.update.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Warn("request body is empty")
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			log.Warn("failed to decode request", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request"))
			return
		}

		if req.Points == nil && req.Level == nil && req.TotalPomodoros == nil &&
			req.TotalBurntTasks == nil && req.CurrentStreak == nil && req.BestStreak == nil {
			log.Warn("no fields provided for update")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("no fields to update"))
			return
		}

		// 4. Map to domain model
		stats := req.ToDomain()

		log.Debug("updating stats", slog.Any("payload", stats))

		// 5. Call repository/service
		if err := userStatsUpdater.UpdateUserStats(r.Context(), userID, stats); err != nil {
			log.Error("failed to update stats", sl.Err(err))

			// Can add specific DB error handling if needed
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to update stats"))
			return
		}

		log.Info("stats updated successfully")

		// 6. Response
		// Return 204 No Content (success, but no body)
		// or 200 OK with message. For UPDATE, 204 is often used.
		render.Status(r, http.StatusNoContent)
		// If body is needed:
		// render.JSON(w, r, resp.OK())

	}
}
