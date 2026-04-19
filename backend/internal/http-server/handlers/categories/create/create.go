package create

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	Name string `json:"name" validate:"required"`
}

type Response struct {
	resp.Response
	ID int64 `json:"id,omitempty"`
}

type CategoryCreator interface {
	CreateCategory(ctx context.Context, category domain.Category) (int64, error)
}

func (r Request) ToDomain(userID int64) domain.Category {
	return domain.Category{
		UserID: userID,
		Name:   r.Name,
	}
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

var validate = validator.New()

func New(log *slog.Logger, categoryCreator CategoryCreator, eventLogger EventLogger) http.HandlerFunc {
	const op = "handlers.categories.create.New"
	return func(w http.ResponseWriter, r *http.Request) {
		l, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}
		var req Request

		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				l.Warn("request body is empty")

				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			l.Warn("failed to decode request body", sl.Err(err))

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err = validate.Struct(req); err != nil {
			var validateErr validator.ValidationErrors

			if errors.As(err, &validateErr) {
				l.Warn("invalid request", sl.Err(err))

				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.ValidationError(validateErr))
				return
			}
			l.Error("internal validation error", sl.Err(err))

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal server error"))
			return
		}

		l.Debug("request body decoded", slog.Any("request", req))
		category := req.ToDomain(userID)
		id, err := categoryCreator.CreateCategory(r.Context(), category)
		if err != nil {
			if errors.Is(err, storage.ErrCategoryExists) {
				l.Warn("category already exists", sl.Err(err))

				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("category already exists"))
				return
			}
			l.Error("failed to create category", sl.Err(err))

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to create category"))
			return
		}

		l.Info("category created", slog.Int64("id", id))

		if eventLogger != nil {
			eventLogger.LogEvent(userID, "CATEGORY_CREATED", id, map[string]any{
				"name": req.Name,
			})
		}

		render.Status(r, http.StatusCreated)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			ID:       id,
		})
	}
}
