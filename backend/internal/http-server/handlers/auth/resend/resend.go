package resend

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
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

func sendVerificationWebhook(ctx context.Context, webhookURL, webhookSecret, email, token string) error {
	const op = "handlers.auth.resend.sendVerificationWebhook"

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
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to get user"}, apiresponse.Compatibility{})
			return
		}

		if user.IsVerified {
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusBadRequest, Code: apiresponse.ValidationError, Message: "user is already verified"}, apiresponse.Compatibility{})
			return
		}

		token, err := generateToken()
		if err != nil {
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to generate token"}, apiresponse.Compatibility{})
			return
		}
		err = resender.SaveEmailVerificationToken(r.Context(), userID, token, time.Now().Add(24*time.Hour))
		if err != nil {
			log.Error("failed to save token", sl.Err(err))
			apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusInternalServerError, Code: apiresponse.InternalError, Message: "failed to save token"}, apiresponse.Compatibility{})
			return
		}

		log.Info("verification email queued", slog.Int64("user_id", userID))

		render.JSON(w, r, resp.OK())
	}
}
