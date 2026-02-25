package storage

import (
	"context"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

type Storage interface {
	GetPendingTasksWithDeadline(ctx context.Context) ([]domain.Task, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	GetTasksDueBetween(ctx context.Context, from, to time.Time) ([]domain.Task, error)
}
