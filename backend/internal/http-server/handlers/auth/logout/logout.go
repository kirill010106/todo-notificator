package logout

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
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
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			log.Warn("failed to decode request body")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err := validate.Struct(req); err != nil {
			log.Warn("invalid request", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("validation error"))
			return
		}

		if req.AllDevices && hasUserID {
			err = tokenRevoker.DeleteUserRefreshTokens(r.Context(), userID)
			if err != nil {
				log.Error("failed to delete all user tokens", sl.Err(err))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, resp.Error("failed to logout from all devices"))
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
					render.Status(r, http.StatusInternalServerError)
					render.JSON(w, r, resp.Error("failed to logout"))
					return
				}
			}
			log.Info("logged out successfully")
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
