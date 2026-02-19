package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/render"
	"github.com/golang-jwt/jwt/v5"
	resp "github.com/kirill010106/todo-notificator/backend/internal/lib/api/response"
)

type contextKey string

const userIDKey contextKey = "user_id"

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

			claims := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(secret), nil
			})

			if err != nil || !token.Valid {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("invalid token"))
				return
			}

			uid, ok := claims["uid"].(float64)
			if !ok {
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("invalid token claims"))
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, int64(uid))

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) (int64, bool) {
	userID, zbs := ctx.Value(userIDKey).(int64)
	return userID, zbs
}
