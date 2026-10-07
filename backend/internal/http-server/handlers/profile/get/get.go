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

type ProfileProvider interface {
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	CountCategories(ctx context.Context, userID int64) (int, error)
	CountActiveReminders(ctx context.Context, userID int64) (int, error)
}

type ProfileData struct {
	ID                   int64  `json:"id"`
	Email                string `json:"email"`
	IsVerified           bool   `json:"is_verified"`
	IsPremium            bool   `json:"is_premium"`
	CategoriesCount      int    `json:"categories_count"`
	ActiveRemindersCount int    `json:"active_reminders_count"`
	CategoriesLimit      int    `json:"categories_limit"`
	RemindersLimit       int    `json:"reminders_limit"`
}

type Response struct {
	resp.Response
	Profile ProfileData `json:"profile"`
}

func New(log *slog.Logger, provider ProfileProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.profile.get.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		user, err := provider.GetUserByID(r.Context(), userID)
		if err != nil {
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get profile"))
			return
		}

		catCount, _ := provider.CountCategories(r.Context(), userID)
		remCount, _ := provider.CountActiveReminders(r.Context(), userID)

		catLimit := 3
		remLimit := 3
		if user.IsPremium {
			catLimit = -1
			remLimit = -1
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Profile: ProfileData{
				ID:                   user.ID,
				Email:                user.Email,
				IsVerified:           user.IsVerified,
				IsPremium:            user.IsPremium,
				CategoriesCount:      catCount,
				ActiveRemindersCount: remCount,
				CategoriesLimit:      catLimit,
				RemindersLimit:       remLimit,
			},
		})
	}
}
