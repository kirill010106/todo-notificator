package domain

import "time"

type User struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	PassHash   []byte `json:"-"`
	IsVerified bool   `json:"is_verified"`
}

type EmailVerificationToken struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
