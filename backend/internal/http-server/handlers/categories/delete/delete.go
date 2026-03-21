package delete

import "context"

type CategoryDeleter interface {
	CategoryDeleter(ctx context.Context, userID int64, categoryID int64) error
}
