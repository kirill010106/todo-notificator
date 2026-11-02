package getone

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type CategoryGetter interface {
	GetCategory(ctx context.Context, userID int64, categoryID int64) (domain.Category, error)
}

type Response struct {
	resp.Response
	Category domain.Category `json:"category"`
}

func New(log *slog.Logger, categoryGetter CategoryGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.categories.getone.New"

		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		categoryIDStr := chi.URLParam(r, "category_id")
		categoryID, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err != nil || categoryID <= 0 {
			l.Warn("invalid category id", slog.String("category_id", categoryIDStr))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "invalid category id"}, apiresponse.Compatibility{})
			return
		}

		category, err := categoryGetter.GetCategory(r.Context(), userID, categoryID)
		if err != nil {
			if errors.Is(err, storage.ErrCategoryNotFound) {
				l.Info("category not found", slog.Int64("category_id", categoryID))
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "category not found"}, apiresponse.Compatibility{})
				return
			}
			l.Error("failed to get category", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to get category"}, apiresponse.Compatibility{})
			return
		}

		l.Info("category retrieved", slog.Int64("category_id", categoryID))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			Category: category,
		})
	}
}
