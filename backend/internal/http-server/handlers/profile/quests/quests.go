package quests

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

type Provider interface {
	GetDailyQuests(ctx context.Context, userID int64) ([]domain.DailyQuest, error)
}

type Response struct {
	resp.Response
	Quests []domain.DailyQuest `json:"quests"`
}

func New(log *slog.Logger, provider Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.profile.quests.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		list, err := provider.GetDailyQuests(r.Context(), userID)
		if err != nil {
			log.Error("failed to get daily quests", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get daily quests"))
			return
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Quests:   list,
		})
	}
}
