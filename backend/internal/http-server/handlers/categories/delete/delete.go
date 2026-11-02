package delete

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type CategoryDeleter interface {
	DeleteCategory(ctx context.Context, userID int64, categoryID int64) error
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

func New(log *slog.Logger, categoryDeleter CategoryDeleter, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.categories.delete.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		categoryIDStr := chi.URLParam(r, "category_id")
		if categoryIDStr == "" {
			log.Warn("category_id is empty")
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "category_id is required"}, apiresponse.Compatibility{})
			return
		}

		categoryID, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err != nil {
			log.Warn("invalid category id", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "invalid category_id"}, apiresponse.Compatibility{})
			return
		}

		log = log.With(slog.Int64("category_id", categoryID))

		err = categoryDeleter.DeleteCategory(r.Context(), userID, categoryID)
		if err != nil {
			if errors.Is(err, storage.ErrCategoryNotFound) {
				log.Info("category not found or access denied")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "category not found"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to delete category", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal error"}, apiresponse.Compatibility{})
			return
		}

		log.Info("category deleted successfully")

		if eventLogger != nil {
			eventLogger.LogEvent(userID, "CATEGORY_DELETED", categoryID, nil)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
