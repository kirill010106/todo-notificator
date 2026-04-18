package refresh

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type Response struct {
	resp.Response
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type TokenRefresher interface {
	RotateRefreshToken(ctx context.Context, oldToken string, newToken string, expiresAt time.Time) (*domain.User, error)
}

var validate = validator.New()

func New(log *slog.Logger, tokenRefresher TokenRefresher, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.refresh.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Info("request body is empty")
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("empty request body"))
				return
			}
			log.Warn("failed to decode request body", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err := validate.Struct(req); err != nil {
			var validateErrs validator.ValidationErrors
			if errors.As(err, &validateErrs) {
				log.Warn("invalid request", sl.Err(err))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.ValidationError(validateErrs))
				return
			}
			log.Error("internal validation error", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request"))
			return
		}

		newRefreshToken, err := jwt.NewRefreshToken()
		if err != nil {
			log.Error("failed to generate new refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to generate token"))
			return
		}

		expiresAt := time.Now().Add(cfg.RefreshTokenTTL)

		user, err := tokenRefresher.RotateRefreshToken(r.Context(), req.RefreshToken, newRefreshToken, expiresAt)
		if err != nil {
			if errors.Is(err, storage.ErrRefreshTokenInvalid) {
				log.Info("invalid refresh token")
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("invalid or expired refresh token"))
				return
			}
			log.Error("failed to rotate refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal server error"))
			return
		}

		log = log.With(slog.Int64("user_id", user.ID))

		accessToken, err := jwt.NewAccessToken(*user, cfg.AppSecret, cfg.AccessTokenTTL)
		if err != nil {
			log.Error("failed to generate access token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to generate token"))
			return
		}

		log.Info("tokens refreshed successfully")

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:     resp.OK(),
			AccessToken:  accessToken,
			RefreshToken: newRefreshToken,
			ExpiresIn:    int64(cfg.AccessTokenTTL.Seconds()),
		})
	}
}
