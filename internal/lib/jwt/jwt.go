package jwt

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"time"
)

func NewToken(user domain.User, secret string, duration time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(duration).Unix(),
	})

	return token.SignedString([]byte(secret))
}
