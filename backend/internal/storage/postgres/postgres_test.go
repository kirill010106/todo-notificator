package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
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
SELECT id, user_id, title, description, deadline, status, is_notified
FROM tasks
WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`)

	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(dataQuery).
		WithArgs(userID, limit, offset).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "user_id", "title", "description", "deadline", "status", "is_notified"}).
				AddRow(int64(10), userID, "T1", "D1", now, "pending", false).
				AddRow(int64(9), userID, "T2", "D2", nil, "done", true),
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
SELECT id, user_id, title, description, deadline, status, is_notified
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
