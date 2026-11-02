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
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Request struct {
	RefreshToken string `json:"refresh_token" validate:"required,max=64"`
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
		err := helpers.DecodeAuthJSON(w, r, &req)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusRequestEntityTooLarge, Code: apiresponse.PayloadTooLarge, Message: "request body exceeds 4096 bytes"}, apiresponse.Compatibility{})
				return
			}
			if errors.Is(err, io.EOF) {
				log.Info("request body is empty")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "empty request body"}, apiresponse.Compatibility{})
				return
			}
			log.Warn("failed to decode request body", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "failed to decode request"}, apiresponse.Compatibility{})
			return
		}

		if len(req.RefreshToken) > 64 {
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "refresh token exceeds 64 bytes"}, apiresponse.Compatibility{})
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
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "invalid request"}, apiresponse.Compatibility{})
			return
		}

		newRefreshToken, err := jwt.NewRefreshToken()
		if err != nil {
			log.Error("failed to generate new refresh token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to generate token"}, apiresponse.Compatibility{})
			return
		}

		expiresAt := time.Now().Add(cfg.RefreshTokenTTL)

		user, err := tokenRefresher.RotateRefreshToken(r.Context(), req.RefreshToken, newRefreshToken, expiresAt)
		if err != nil {
			if errors.Is(err, storage.ErrRefreshTokenInvalid) {
				log.Info("invalid refresh token")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "invalid or expired refresh token"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to rotate refresh token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal server error"}, apiresponse.Compatibility{})
			return
		}

		log = log.With(slog.Int64("user_id", user.ID))

		accessToken, err := jwt.NewAccessToken(*user, cfg.AppSecret, cfg.AccessTokenTTL)
		if err != nil {
			log.Error("failed to generate access token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to generate token"}, apiresponse.Compatibility{})
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
