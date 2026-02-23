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
	ExpiresIn    int64  `json:"expires_at"`
}

type TokenRefresher interface {
	GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	SaveRefreshToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error
	DeleteRefreshToken(ctx context.Context, token string) error
}

var validate = validator.New()

func New(log *slog.Logger, tokenRefresher TokenRefresher, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.refresh.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Info("request body is empty")
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, "empty request body")
				return
			}
			log.Warn("failed to decode request body", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, "failed to decode request")
			return
		}

		if err := validate.Struct(req); err != nil {
			log.Warn("invalid requedst", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, "validation error")
			return
		}

		refreshTokenData, err := tokenRefresher.GetRefreshToken(r.Context(), req.RefreshToken)
		if err != nil {
			if errors.Is(err, storage.ErrRefreshTokenInvalid) {
				log.Info("invalid refresh token")
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, "invalid or expired refresh token")
				return
			}
			log.Error("failed to get refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, "internal server error")
			return
		}

		log = log.With(slog.Int64("user_ID", refreshTokenData.UserID))

		user, err := tokenRefresher.GetUserByID(r.Context(), refreshTokenData.UserID)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				log.Warn("user not found")
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, "user not found")
				return
			}
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, "internal server error")
			return
		}

		err = tokenRefresher.DeleteRefreshToken(r.Context(), req.RefreshToken)
		if err != nil {
			log.Error("failed to delete old refresh token", sl.Err(err))
		}

		accessToken, err := jwt.NewAccessToken(*user, cfg.AppSecret, cfg.AccessTokenTTL)
		if err != nil {
			log.Error("failed to generate access token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, "failed to generate token")
		}

		newRefreshToken, err := jwt.NewRefreshToken()
		if err != nil {
			log.Error("failed to generate refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, "failed to generate token")
			return
		}

		expiresAt := time.Now().Add(cfg.RefreshTokenTTL)
		err = tokenRefresher.SaveRefreshToken(r.Context(), user.ID, newRefreshToken, expiresAt)
		if err != nil {
			log.Error("failed to save refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, "failed to save token")
			return
		}

		log.Info("tokens refreshed succesfully")

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response:     resp.OK(),
			AccessToken:  accessToken,
			RefreshToken: newRefreshToken,
			ExpiresIn:    int64(cfg.AccessTokenTTL.Seconds()),
		})
	}
}
