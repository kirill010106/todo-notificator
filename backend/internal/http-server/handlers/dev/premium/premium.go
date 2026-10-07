package premium

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

type PremiumToggler interface {
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	SetPremiumStatus(ctx context.Context, userID int64, isPremium bool) error
}

type Response struct {
	resp.Response
	IsPremium bool `json:"is_premium"`
}

func New(log *slog.Logger, toggler PremiumToggler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.dev.premium.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		user, err := toggler.GetUserByID(r.Context(), userID)
		if err != nil {
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get user"))
			return
		}

		newStatus := !user.IsPremium
		err = toggler.SetPremiumStatus(r.Context(), userID, newStatus)
		if err != nil {
			log.Error("failed to update premium status", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to update premium status"))
			return
		}

		log.Info("toggled premium status", slog.Int64("user_id", userID), slog.Bool("is_premium", newStatus))

		render.JSON(w, r, Response{
			Response:  resp.OK(),
			IsPremium: newStatus,
		})
	}
}
