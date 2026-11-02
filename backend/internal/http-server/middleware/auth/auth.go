package auth

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"net/http"
	"strings"

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
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "unauthorized"}, apiresponse.Compatibility{})
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "invalid auth header format"}, apiresponse.Compatibility{})
				return
			}

			tokenStr := parts[1]

			claims, err := j.ParseAccessToken(tokenStr, secret)
			if err != nil {
				apiresponse.Write(w, r, apiresponse.Problem{Status: http.StatusUnauthorized, Code: apiresponse.Unauthorized, Message: "invalid token"}, apiresponse.Compatibility{})
				return
			}

			if len(providers) > 0 {
				user, err := providers[0].GetUserByID(r.Context(), claims.UserID)
				if err != nil {
					if errors.Is(err, storage.ErrUserNotFound) {
						apiresponse.Write(w, r, apiresponse.Problem{Status: 401, Code: apiresponse.Unauthorized, Message: "account not found"}, apiresponse.Compatibility{})
						return
					}
					apiresponse.Write(w, r, apiresponse.Problem{Status: 503, Code: apiresponse.AccountUnavailable, Message: "cannot load current account"}, apiresponse.Compatibility{})
					return
				}
				if claims.AuthVersion != user.AuthVersion {
					apiresponse.Write(w, r, apiresponse.Problem{Status: 401, Code: apiresponse.Unauthorized, Message: "Войдите в аккаунт заново"}, apiresponse.Compatibility{})
					return
				}
				claims.IsPremium = user.IsPremium
				claims.IsVerified = user.IsVerified
			}
			ctx := context.WithValue(r.Context(), contextKey("auth_version"), claims.AuthVersion)
			ctx = context.WithValue(ctx, userIDKey, claims.UserID)
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

func GetAuthVersion(ctx context.Context) int64 {
	v, _ := ctx.Value(contextKey("auth_version")).(int64)
	return v
}
