package domain

import "time"

type RefreshToken struct {
	ID        int64     `json:"-"`
	UserID    int64     `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
