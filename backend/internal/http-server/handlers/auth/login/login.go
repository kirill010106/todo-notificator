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
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

type Request struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
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

var validate = validator.New()

func New(log *slog.Logger, userProvider UserProvider, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.login.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request

		if err := render.DecodeJSON(r.Body, &req); err != nil {
			if errors.Is(err, io.EOF) {
				log.Info("request body is empty")
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("request body is empty"))
				return
			}
			log.Error("failed to decode request body", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err := validate.Struct(req); err != nil {
			var validateErrs validator.ValidationErrors
			if errors.As(err, &validateErrs) {
				log.Info("validation failed", sl.Err(err))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("validation failed"))
				return
			}
			log.Error("internal validation error", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request"))
			return
		}

		user, err := userProvider.User(r.Context(), req.Email)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				log.Info("user not found", slog.String("email", req.Email))
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("invalid credentials"))
				return
			}
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		if err := bcrypt.CompareHashAndPassword(user.PassHash, []byte(req.Password)); err != nil {
			log.Info("invalid password", slog.String("email", req.Email))
			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, resp.Error("invalid credentials"))
			return
		}

		accessToken, err := jwt.NewAccessToken(user, cfg.AppSecret, cfg.AccessTokenTTL)
		if err != nil {
			log.Error("failed to generate access token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		refreshToken, err := jwt.NewRefreshToken()
		if err != nil {
			log.Error("failed to generate refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to generate token"))
			return
		}

		expiresAt := time.Now().Add(cfg.RefreshTokenTTL)
		err = userProvider.SaveRefreshToken(r.Context(), user.ID, refreshToken, expiresAt)
		if err != nil {
			log.Error("failed to save refresh token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to save token"))
			return
		}

		log.Info("user logged in successfully",
			slog.Int64("id", user.ID),
			slog.String("email", user.Email),
		)

		render.JSON(w, r, Response{
			Response:     resp.OK(),
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresIn:    int64(cfg.AccessTokenTTL.Seconds()),
		})
	}
}
