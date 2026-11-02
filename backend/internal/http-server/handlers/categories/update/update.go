package update

import (
	"context"
	"errors"
	"io"
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

type Request struct {
	Name string `json:"name"`
}

type Response struct {
	resp.Response
	CategoryID int64 `json:"category_id"`
}

type CategoryUpdater interface {
	UpdateCategory(ctx context.Context, userID int64, categoryID int64, c domain.CategoryUpdate) error
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

func (r Request) ToDomain() domain.CategoryUpdate {
	return domain.CategoryUpdate{
		Name: &r.Name,
	}
}

func New(log *slog.Logger, categoryUpdater CategoryUpdater, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.categories.update.New"

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

		var req Request

		err = render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Warn("request body is empty")

				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "empty request body"}, apiresponse.Compatibility{})
				return
			}
			log.Warn("failed to decode request body", sl.Err(err))

			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "failed to decode request"}, apiresponse.Compatibility{})
			return
		}

		if req.Name == "" {
			log.Warn("no fields to update")
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "at least one field must be provided"}, apiresponse.Compatibility{})
			return
		}

		log.Debug("request body decoded", slog.Any("request", req))

		categoryUpdate := req.ToDomain()
		err = categoryUpdater.UpdateCategory(r.Context(), userID, categoryID, categoryUpdate)

		if err != nil {
			if errors.Is(err, storage.ErrCategoryNotFound) {
				log.Info("category not found or access denied")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusNotFound, Code: apiresponse.NotFound, Message: "category not found"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to update category", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to update category"}, apiresponse.Compatibility{})
			return
		}

		log.Info("category updated successfully")

		if eventLogger != nil {
			eventLogger.LogEvent(userID, "CATEGORY_UPDATED", categoryID, map[string]any{
				"name": req.Name,
			})
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:   resp.OK(),
			CategoryID: categoryID,
		})
	}
}
