package auth

import (
	"context"
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

func New(secret string) func(next http.Handler) http.Handler {
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
