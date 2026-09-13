package register

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

type Request struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type Response struct {
	resp.Response
	UserID int64 `json:"user_id,omitempty"`
}

type UserSaver interface {
	SaveUser(ctx context.Context, email string, passHash []byte) (int64, error)
	SaveEmailVerificationToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type EventLogger interface {
	LogEvent(userID int64, action string, entityID int64, details map[string]any)
}

var validate = validator.New()

func New(log *slog.Logger, userSaver UserSaver, eventLogger EventLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.register.New"

		var req Request
		if err := render.DecodeJSON(r.Body, &req); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err := validate.Struct(req); err != nil {
			var validateErrs validator.ValidationErrors
			if errors.As(err, &validateErrs) {
				log.Info("validation failed", sl.Err(err))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.ValidationError(validateErrs))
				return
			}
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request data"))
			return
		}

		passHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Error("failed to generate password hash", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		id, err := userSaver.SaveUser(r.Context(), req.Email, passHash)
		if err != nil {
			if errors.Is(err, storage.ErrUserExists) {
				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("user already exists"))
				return
			}
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to save user"))
			return
		}

		token, err := generateToken()
		if err != nil {
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to generate token"))
			return
		}
		err = userSaver.SaveEmailVerificationToken(r.Context(), id, token, time.Now().Add(24*time.Hour))
		if err != nil {
			log.Error("failed to save token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to save token"))
			return
		}

		if eventLogger != nil {
			eventLogger.LogEvent(id, "USER_REGISTERED", id, map[string]any{
				"email": req.Email,
			})
		}

		render.JSON(w, r, Response{
			Response: resp.OK(),
			UserID:   id,
		})
	}
}
