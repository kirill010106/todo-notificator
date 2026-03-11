package register

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
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
}

var validate = validator.New()

func New(log *slog.Logger, userSaver UserSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.register.New"

		var req Request
		if err := render.DecodeJSON(r.Body, &req); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("failed to decode request"))
			return
		}

		if err := validate.Struct(req); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("invalid request data"))
			return
		}

		passHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Error("failed to generate password hash", slog.String("error", err.Error()))
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

		render.JSON(w, r, Response{
			Response: resp.OK(),
			UserID:   id,
		})
	}
}
