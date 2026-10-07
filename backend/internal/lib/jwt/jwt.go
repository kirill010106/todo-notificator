package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
)

func NewAccessToken(user domain.User, secret string, duration time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":         user.ID,
		"email":       user.Email,
		"type":        "access",
		"exp":         time.Now().Add(duration).Unix(),
		"iat":         time.Now().Unix(),
		"is_verified": user.IsVerified,
		"is_premium":  user.IsPremium,
	})

	return token.SignedString([]byte(secret))
}

func NewRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

type UserClaims struct {
	UserID     int64
	Email      string
	IsVerified bool
	IsPremium  bool
}

func ParseAccessToken(tokenString string, secret string) (*UserClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != "access" {
		return nil, fmt.Errorf("invalid token type")
	}

	uid, ok := claims["uid"].(float64)
	if !ok {
		return nil, fmt.Errorf("uid not found in token")
	}

	email, _ := claims["email"].(string)
	isVerified, _ := claims["is_verified"].(bool)
	isPremium, _ := claims["is_premium"].(bool)

	return &UserClaims{
		UserID:     int64(uid),
		Email:      email,
		IsVerified: isVerified,
		IsPremium:  isPremium,
	}, nil
}
