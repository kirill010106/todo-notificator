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
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
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

				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "empty request body"}, apiresponse.Compatibility{})
				return
			}
			l.Warn("failed to decode request body", sl.Err(err))

			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "failed to decode request"}, apiresponse.Compatibility{})
			return
		}

		if err = validate.Struct(req); err != nil {
			var validateErr validator.ValidationErrors

			if errors.As(err, &validateErr) {
				l.Warn("invalid request", sl.Err(err))

				helpers.RenderValidationError(w, r, validateErr)
				return
			}
			l.Error("internal validation error", sl.Err(err))

			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal server error"}, apiresponse.Compatibility{})
			return
		}

		l.Debug("request body decoded", slog.Any("request", req))
		category := req.ToDomain(userID)
		id, err := categoryCreator.CreateCategory(r.Context(), category)
		if err != nil {
			if errors.Is(err, storage.ErrCategoryLimitReached) {
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusForbidden, Code: apiresponse.CategoryLimitReached, Message: "В бесплатном тарифе доступно не более 3 категорий. Перейдите на Премиум для создания неограниченного числа категорий"}, apiresponse.Compatibility{})
				return
			}
			if errors.Is(err, storage.ErrCategoryExists) {
				l.Warn("category already exists", sl.Err(err))

				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusConflict, Code: apiresponse.Conflict, Message: "category already exists"}, apiresponse.Compatibility{})
				return
			}
			l.Error("failed to create category", sl.Err(err))

			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to create category"}, apiresponse.Compatibility{})
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
