package logout

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
	AllDevices   bool   `json:"all_devices"`
}

type TokenRevoker interface {
	DeleteRefreshToken(ctx context.Context, token string) error
	DeleteUserRefreshTokens(ctx context.Context, userID int64) error
}

var validate = validator.New()

func New(log *slog.Logger, tokenRevoker TokenRevoker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.logout.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		userID, hasUserID := auth.GetUserID(r.Context())
		if hasUserID {
			log = log.With(slog.Int64("user_id", userID))
		}

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Info("request body is empty")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "empty request body"}, apiresponse.Compatibility{})
				return
			}
			log.Warn("failed to decode request body")
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "failed to decode request"}, apiresponse.Compatibility{})
			return
		}

		if err := validate.Struct(req); err != nil {
			var validateErrs validator.ValidationErrors
			if errors.As(err, &validateErrs) {
				log.Warn("invalid request", sl.Err(err))
				helpers.RenderValidationError(w, r, validateErrs)
				return
			}
			log.Error("internal validation error", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "validation error"}, apiresponse.Compatibility{})
			return
		}

		if req.AllDevices && hasUserID {
			err = tokenRevoker.DeleteUserRefreshTokens(r.Context(), userID)
			if err != nil {
				log.Error("failed to delete all user tokens", sl.Err(err))
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to logout from all devices"}, apiresponse.Compatibility{})
				return
			}
			log.Info("logged out from all devices")
		} else {
			err = tokenRevoker.DeleteRefreshToken(r.Context(), req.RefreshToken)
			if err != nil {
				if errors.Is(err, storage.ErrRefreshTokenInvalid) {
					log.Info("refresh token not found or already expired")
				} else {
					log.Error("failed to delete refresh token", sl.Err(err))
					apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to logout"}, apiresponse.Compatibility{})
					return
				}
			}
			log.Info("logged out successfully")
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
