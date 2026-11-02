package login

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
	"golang.org/x/crypto/bcrypt"
)

type Request struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=8"`
}

type Response struct {
	resp.Response
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type UserProvider interface {
	User(ctx context.Context, email string) (domain.User, error)
	SaveRefreshToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

var validate = validator.New()

func New(log *slog.Logger, userProvider UserProvider, cfg *config.Config, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.login.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request

		if err := helpers.DecodeAuthJSON(w, r, &req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusRequestEntityTooLarge, Code: apiresponse.PayloadTooLarge, Message: "request body exceeds 4096 bytes"}, apiresponse.Compatibility{})
				return
			}
			if errors.Is(err, io.EOF) {
				log.Info("request body is empty")
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "request body is empty"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to decode request body", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "failed to decode request"}, apiresponse.Compatibility{})
			return
		}

		if len(req.Password) > 72 {
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "password exceeds 72 bytes"}, apiresponse.Compatibility{})
			return
		}
		if err := validate.Struct(req); err != nil {
			var validateErrs validator.ValidationErrors
			if errors.As(err, &validateErrs) {
				log.Info("validation failed", sl.Err(err))
				helpers.RenderValidationError(w, r, validateErrs)
				return
			}
			log.Error("internal validation error", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "invalid request"}, apiresponse.Compatibility{})
			return
		}

		user, err := userProvider.User(r.Context(), req.Email)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				log.Info("user not found", slog.String("email", req.Email))
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "invalid credentials"}, apiresponse.Compatibility{})
				return
			}
			log.Error("failed to get user", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal error"}, apiresponse.Compatibility{})
			return
		}

		if err := bcrypt.CompareHashAndPassword(user.PassHash, []byte(req.Password)); err != nil {
			log.Info("invalid password", slog.String("email", req.Email))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "invalid credentials"}, apiresponse.Compatibility{})
			return
		}

		accessToken, err := jwt.NewAccessToken(user, cfg.AppSecret, cfg.AccessTokenTTL)
		if err != nil {
			log.Error("failed to generate access token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "internal error"}, apiresponse.Compatibility{})
			return
		}

		refreshToken, err := jwt.NewRefreshToken()
		if err != nil {
			log.Error("failed to generate refresh token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to generate token"}, apiresponse.Compatibility{})
			return
		}

		expiresAt := time.Now().Add(cfg.RefreshTokenTTL)
		if versioned, ok := userProvider.(interface {
			SaveRefreshTokenVersion(context.Context, int64, int64, string, time.Time) error
		}); ok {
			err = versioned.SaveRefreshTokenVersion(r.Context(), user.ID, user.AuthVersion, refreshToken, expiresAt)
		} else {
			err = userProvider.SaveRefreshToken(r.Context(), user.ID, refreshToken, expiresAt)
		}
		if err != nil {
			log.Error("failed to save refresh token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to save token"}, apiresponse.Compatibility{})
			return
		}

		log.Info("user logged in successfully",
			slog.Int64("id", user.ID),
			slog.String("email", user.Email),
		)

		if eventLogger != nil {
			eventLogger.LogEvent(user.ID, "USER_LOGGED_IN", user.ID, map[string]any{
				"email": user.Email,
			})
		}

		render.JSON(w, r, Response{
			Response:     resp.OK(),
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresIn:    int64(cfg.AccessTokenTTL.Seconds()),
		})
	}
}
