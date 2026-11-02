package get

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
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
	Categories []domain.Category `json:"categories"`
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
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "categories not found"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to get categories", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to get categories"}, apiresponse.Compatibility{})
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
