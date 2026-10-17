package storage

import (
	"context"
	"errors"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

var (
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskExists          = errors.New("task already exists")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserExists          = errors.New("user already exists")
	ErrRefreshTokenInvalid = errors.New("refresh token invalid or expired")
)

type Storage interface {
	ClaimReminder(context.Context) (*domain.ReminderDelivery, error)
	ReminderReady(context.Context, domain.ReminderDelivery) (bool, error)
	FinishReminder(context.Context, domain.ReminderDelivery, error) error
	NextReminderAt(context.Context) (*time.Time, error)
}
