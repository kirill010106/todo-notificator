package verify

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Response struct {
	resp.Response
}

// EmailVerifier contains required methods from Storage
type EmailVerifier interface {
	GetEmailVerificationToken(ctx context.Context, token string) (domain.EmailVerificationToken, error)
	VerifyUserEmail(ctx context.Context, userID int64) error
	DeleteEmailVerificationToken(ctx context.Context, token string) error
}

func New(log *slog.Logger, emailVerifier EmailVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.auth.verify.New"

		log := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		token := r.URL.Query().Get("token")

		if token == "" {
			log.Warn("empty token")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("token is required"))
			return
		}

		tokenInfo, err := emailVerifier.GetEmailVerificationToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, storage.ErrTokenNotFound) {
				log.Warn("token not found", slog.String("token", token))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, resp.Error("invalid or expired token"))
				return
			}
			log.Error("failed to get token", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		if time.Now().After(tokenInfo.ExpiresAt) {
			log.Warn("token expired", slog.String("token", token))
			emailVerifier.DeleteEmailVerificationToken(r.Context(), token)
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("token expired"))
			return
		}

		if err := emailVerifier.VerifyUserEmail(r.Context(), tokenInfo.UserID); err != nil {
			log.Error("failed to verify user email", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("internal error"))
			return
		}

		if err := emailVerifier.DeleteEmailVerificationToken(r.Context(), token); err != nil {
			log.Error("failed to delete used token", sl.Err(err))
		}

		log.Info("email successfully verified", slog.Int64("user_id", tokenInfo.UserID))

		render.JSON(w, r, Response{
			Response: resp.OK(),
		})
	}
}
