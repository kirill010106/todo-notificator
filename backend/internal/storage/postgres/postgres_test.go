package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestGetTasks_SuccessWithPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	userID := int64(42)
	limit := 2
	offset := 4

	countQuery := regexp.QuoteMeta(`
		SELECT COUNT(*) FROM tasks WHERE user_id = $1
	`)
	mock.ExpectQuery(countQuery).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	dataQuery := regexp.QuoteMeta(`
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified
FROM tasks
WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`)

	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(dataQuery).
		WithArgs(userID, limit, offset).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "user_id", "title", "description", "deadline", "reminder_at", "status", "is_notified"}).
				AddRow(int64(10), userID, "T1", "D1", now, now, "pending", false).
				AddRow(int64(9), userID, "T2", "D2", nil, nil, "done", true),
		)

	tasks, total, err := s.GetTasks(context.Background(), userID, limit, offset)
	require.NoError(t, err)
	require.Equal(t, 5, total)
	require.Len(t, tasks, 2)
	require.Equal(t, int64(10), tasks[0].ID)
	require.Equal(t, "T1", tasks[0].Title)
	require.NotNil(t, tasks[0].Deadline)
	require.Equal(t, "done", tasks[1].Status)
	require.True(t, tasks[1].IsNotified)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTasks_CountQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	countQuery := regexp.QuoteMeta(`
		SELECT COUNT(*) FROM tasks WHERE user_id = $1
	`)
	mock.ExpectQuery(countQuery).
		WithArgs(int64(1)).
		WillReturnError(errors.New("count failed"))

	tasks, total, err := s.GetTasks(context.Background(), 1, 10, 0)
	require.Error(t, err)
	require.Nil(t, tasks)
	require.Zero(t, total)
	require.Contains(t, err.Error(), "count failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTasks_DataQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	userID := int64(7)

	countQuery := regexp.QuoteMeta(`
		SELECT COUNT(*) FROM tasks WHERE user_id = $1
	`)
	mock.ExpectQuery(countQuery).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	dataQuery := regexp.QuoteMeta(`
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified
FROM tasks
WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`)
	mock.ExpectQuery(dataQuery).
		WithArgs(userID, 10, 0).
		WillReturnError(errors.New("data failed"))

	tasks, total, err := s.GetTasks(context.Background(), userID, 10, 0)
	require.Error(t, err)
	require.Nil(t, tasks)
	require.Zero(t, total)
	require.Contains(t, err.Error(), "data failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateCategory_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	category := domain.Category{UserID: 5, Name: "Work"}

	query := regexp.QuoteMeta(`
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		RETURNING id
	`)

	mock.ExpectQuery(query).
		WithArgs(category.UserID, category.Name).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))

	id, err := s.CreateCategory(context.Background(), category)
	require.NoError(t, err)
	require.Equal(t, int64(11), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateCategory_Conflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	query := regexp.QuoteMeta(`
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		RETURNING id
	`)

	mock.ExpectQuery(query).
		WithArgs(int64(5), "Work").
		WillReturnError(&pgconn.PgError{Code: pgerrcode.UniqueViolation})

	_, err = s.CreateCategory(context.Background(), domain.Category{UserID: 5, Name: "Work"})
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrCategoryExists)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetCategories_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	query := regexp.QuoteMeta(`
		SELECT id, user_id, name  FROM categories
		WHERE user_id = $1
	`)

	mock.ExpectQuery(query).
		WithArgs(int64(7)).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "user_id", "name"}).
				AddRow(int64(1), int64(7), "Work").
				AddRow(int64(2), int64(7), "Health"),
		)

	items, err := s.GetCategories(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "Work", items[0].Name)
	require.Equal(t, "Health", items[1].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteCategory_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	query := regexp.QuoteMeta(`
		DELETE FROM categories
		WHERE user_id = $1 AND id=$2
		RETURNING id
	`)

	mock.ExpectQuery(query).
		WithArgs(int64(3), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))

	err = s.DeleteCategory(context.Background(), 3, 10)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteCategory_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	query := regexp.QuoteMeta(`
		DELETE FROM categories
		WHERE user_id = $1 AND id=$2
		RETURNING id
	`)

	mock.ExpectQuery(query).
		WithArgs(int64(3), int64(10)).
		WillReturnError(sql.ErrNoRows)

	err = s.DeleteCategory(context.Background(), 3, 10)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrCategoryNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCategory_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	name := "Urgent"

	query := regexp.QuoteMeta(`
			UPDATE categories
			SET name = $1
			WHERE user_id = $2 AND id = $3
`)

	mock.ExpectExec(query).
		WithArgs(name, int64(4), int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.UpdateCategory(context.Background(), 4, 12, domain.CategoryUpdate{Name: &name})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCategory_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{Db: db}

	name := "Urgent"

	query := regexp.QuoteMeta(`
			UPDATE categories
			SET name = $1
			WHERE user_id = $2 AND id = $3
`)

	mock.ExpectExec(query).
		WithArgs(name, int64(4), int64(12)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = s.UpdateCategory(context.Background(), 4, 12, domain.CategoryUpdate{Name: &name})
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrCategoryNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
