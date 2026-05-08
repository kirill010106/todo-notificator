package active

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type PomodoroGetter interface {
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
}

type Response struct {
	resp.Response
	ActiveSession *domain.PomodoroSession `json:"active_session,omitempty"`
}

func New(log *slog.Logger, pomodoroGetter PomodoroGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.pomodoros.active.New"

		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		session, err := pomodoroGetter.GetActivePomodoroSession(r.Context(), userID)
		if err != nil {
			if errors.Is(err, storage.ErrSessionNotFound) {
				l.Info("no active pomodoro session found")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("session not found"))
				return
			}

			l.Error("failed to get active pomodoro session", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal server error"))
			return
		}

		l.Info("active pomodoro session retrieved successfully", slog.Int64("session_id", session.ID))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:      resp.OK(),
			ActiveSession: session,
		})
	}
}
