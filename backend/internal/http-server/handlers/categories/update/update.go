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

func (r Request) ToDomain() domain.CategoryUpdate {
	return domain.CategoryUpdate{
		Name: &r.Name,
	}
}

func New(log *slog.Logger, categoryUpdater CategoryUpdater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.categories.update.New"

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

		var req Request

		err = render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Warn("request body is empty")

				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			log.Warn("failed to decode request body", sl.Err(err))

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if req.Name == "" {
			log.Warn("no fields to update")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("at least one field must be provided"))
			return
		}

		log.Debug("request body decoded", slog.Any("request", req))

		categoryUpdate := req.ToDomain()
		err = categoryUpdater.UpdateCategory(r.Context(), userID, categoryID, categoryUpdate)

		if err != nil {
			if errors.Is(err, storage.ErrCategoryNotFound) {
				log.Info("category not found or access denied")
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("category not found"))
				return
			}
			log.Error("failed to update category", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to update category"))
			return
		}

		log.Info("category updated successfully")

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:   resp.OK(),
			CategoryID: categoryID,
		})
	}
}
