package storage

import "errors"

var (
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskExists          = errors.New("task already exists")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserExists          = errors.New("user already exists")
	ErrRefreshTokenInvalid = errors.New("refresh token invalid or expired")
	ErrCategoryExists      = errors.New("category already exists")
	ErrCategoryNotFound    = errors.New("category not found")
	ErrSessionActive       = errors.New("session already active")
	ErrSessionNotFound     = errors.New("session not found")
	ErrBreakExhausted      = errors.New("breaks exhausted")
)
