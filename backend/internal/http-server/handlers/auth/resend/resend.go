package resend

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type TokenResender interface {
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	SaveEmailVerificationToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func New(log *slog.Logger, resender TokenResender, webhookURL, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.resend.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		user, err := resender.GetUserByID(r.Context(), userID)
		if err != nil {
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to get user"))
			return
		}

		if user.IsVerified {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("user is already verified"))
			return
		}

		token, _ := generateToken()
		err = resender.SaveEmailVerificationToken(r.Context(), userID, token, time.Now().Add(24*time.Hour))
		if err != nil {
			log.Error("failed to save token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to save token"))
			return
		}

		go func(email, token string) {
			payload := map[string]string{
				"type":  "verification",
				"email": email,
				"token": token,
			}
			body, _ := json.Marshal(payload)

			reqW, _ := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(body))
			reqW.Header.Set("Content-Type", "application/json")
			reqW.Header.Set("X-Webhook-Secret", webhookSecret)

			client := &http.Client{Timeout: 5 * time.Second}
			client.Do(reqW)
		}(user.Email, token)

		render.JSON(w, r, resp.OK())
	}
}
