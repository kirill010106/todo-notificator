package auth

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"net/http"
	"strings"

	"github.com/go-chi/render"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	j "github.com/kirill010106/todo-notificator/internal/lib/jwt"
)

type contextKey string

const userIDKey contextKey = "user_id"
const isPremiumKey contextKey = "is_premium"
const isVerifiedKey contextKey = "is_verified"

type UserProvider interface {
	GetUserByID(context.Context, int64) (*domain.User, error)
}

func New(secret string, providers ...UserProvider) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("unauthorized"))
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("invalid auth header format"))
				return
			}

			tokenStr := parts[1]

			claims, err := j.ParseAccessToken(tokenStr, secret)
			if err != nil {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("invalid token"))
				return
			}

			if len(providers) > 0 {
				user, err := providers[0].GetUserByID(r.Context(), claims.UserID)
				if err != nil {
					if errors.Is(err, storage.ErrUserNotFound) {
						render.Status(r, 401)
						render.JSON(w, r, resp.ErrorCode("account not found", "UNAUTHORIZED"))
						return
					}
					render.Status(r, 503)
					render.JSON(w, r, resp.ErrorCode("cannot load current account", "ACCOUNT_UNAVAILABLE"))
					return
				}
				claims.IsPremium = user.IsPremium
				claims.IsVerified = user.IsVerified
			}
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, isPremiumKey, claims.IsPremium)
			ctx = context.WithValue(ctx, isVerifiedKey, claims.IsVerified)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey).(int64)
	return userID, ok
}

func GetPremiumStatus(ctx context.Context) (bool, bool) {
	isPremium, ok := ctx.Value(isPremiumKey).(bool)
	return isPremium, ok
}

func GetVerificationStatus(ctx context.Context) (bool, bool) {
	isVerified, ok := ctx.Value(isVerifiedKey).(bool)
	return isVerified, ok
}
