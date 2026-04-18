package register

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func sendVerificationWebhook(ctx context.Context, webhookURL, webhookSecret, email, token string) error {
	const op = "handlers.auth.register.sendVerificationWebhook"

	payload := map[string]string{
		"type":  "verification",
		"email": email,
		"token": token,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%s: marshal payload: %w", op, err)
	}

	reqW, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("%s: create request: %w", op, err)
	}

	reqW.Header.Set("Content-Type", "application/json")
	reqW.Header.Set("X-Webhook-Secret", webhookSecret)

	client := &http.Client{Timeout: 5 * time.Second}
	respW, err := client.Do(reqW)
	if err != nil {
		return fmt.Errorf("%s: send request: %w", op, err)
	}
	defer respW.Body.Close()

	if respW.StatusCode < http.StatusOK || respW.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s: unexpected status code: %d", op, respW.StatusCode)
	}

	return nil
}

var validate = validator.New()

func New(log *slog.Logger, userSaver UserSaver, webhookURL, webhookSecret string) http.HandlerFunc {
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

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := sendVerificationWebhook(ctx, webhookURL, webhookSecret, req.Email, token); err != nil {
			log.Error("failed to send verification webhook", sl.Err(err))
			render.Status(r, http.StatusBadGateway)
			render.JSON(w, r, resp.Error("user created, but failed to send verification email"))
			return
		}

		render.JSON(w, r, Response{
			Response: resp.OK(),
			UserID:   id,
		})
	}
}
