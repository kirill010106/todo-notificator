package delete

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type CategoryDeleter interface {
	DeleteCategory(ctx context.Context, userID int64, categoryID int64) error
}

func New(log *slog.Logger, categoryDeleter CategoryDeleter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.categories.delete.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		categoryIDStr := chi.URLParam(r, "category_id")
		if categoryIDStr == "" {
			log.Warn("category_id is empty")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("category_id is required"))
			return
		}

		categoryID, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err != nil {
			log.Warn("invalid category id", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid category_id"))
			return
		}

		log = log.With(slog.Int64("category_id", categoryID))

		err = categoryDeleter.DeleteCategory(r.Context(), userID, categoryID)
		if err != nil {
			if errors.Is(err, storage.ErrCategoryNotFound) {
				log.Info("category not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("category not found"))
				return
			}
			log.Error("failed to delete category", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		log.Info("category deleted successfully")

		w.WriteHeader(http.StatusNoContent)
	}
}
