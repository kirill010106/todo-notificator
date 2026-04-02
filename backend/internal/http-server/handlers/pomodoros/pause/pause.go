package pause

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type PomodoroProvider interface {
	AddPomodoroBreak(ctx context.Context, sessionID int64) error
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
}

func New(log *slog.Logger, provider PomodoroProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.pomodoros.pause.New"

		log, _, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		sessionIDStr := chi.URLParam(r, "id")
		sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
		if err != nil || sessionID <= 0 {
			log.Warn("invalid session id", slog.String("session_id", sessionIDStr))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid session id"))
			return
		}

		log = log.With(slog.Int64("session_id", sessionID))

		err = provider.AddPomodoroBreak(r.Context(), sessionID)
		if err != nil {
			if errors.Is(err, storage.ErrBreakExhausted) {
				log.Info("user tried to take break but it is exhausted")
				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("extra break is already used"))
				return
			}
			
			if errors.Is(err, storage.ErrSessionNotFound) {
				log.Warn("session not found or not active")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("active session not found"))
				return
			}

			log.Error("failed to add pomodoro break", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to add pomodoro break"))
			return
		}

		log.Info("pomodoro break added successfully")

		render.Status(r, http.StatusOK)
		render.JSON(w, r, resp.OK())
	}
}
