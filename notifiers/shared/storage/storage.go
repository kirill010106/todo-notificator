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
	GetPendingTasksWithUsers(ctx context.Context) ([]domain.TaskWithUser, error)
	GetTasksDueBetween(ctx context.Context, from, to time.Time) ([]domain.Task, error)
}
