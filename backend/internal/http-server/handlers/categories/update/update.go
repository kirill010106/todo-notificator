package update

import (
	"context"

	"github.com/kirill010106/todo-notificator/internal/domain"
)

type CategoryUpdater interface {
	UpdateCategory(ctx context.Context, userID int64, categoryID int64, c domain.CategoryUpdate) error
}
