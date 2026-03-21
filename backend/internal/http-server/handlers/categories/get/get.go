package get

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

type CategoryGetter interface {
	GetCategories(ctx context.Context, userID int64) ([]domain.Category, error)
}

type Response struct {
	resp.Response
	Categories []domain.Category
}

func New(log *slog.Logger, categoryGetter CategoryGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.categories.get.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		categories, err := categoryGetter.GetCategories(r.Context(), userID)

		if err != nil {

			if errors.Is(err, storage.ErrCategoryNotFound) {
				log.Info("categories not found")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("categories not found"))
				return
			}
			log.Error("failed to get categories", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get categories"))
			return
		}

		log.Info("categories retrieved successfully", slog.Int("category_count", len(categories)))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:   resp.OK(),
			Categories: categories,
		})
	}
}
