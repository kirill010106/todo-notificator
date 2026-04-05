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

	s := &Storage{DB: db}

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
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken
FROM tasks
	WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`)

	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(dataQuery).
		WithArgs(userID, limit, offset).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "user_id", "title", "description", "deadline", "reminder_at", "status", "is_notified", "category_id", "pomodoro_taken"}).
				AddRow(int64(10), userID, "T1", "D1", now, now, "pending", false, int64(0), int64(0)).
				AddRow(int64(9), userID, "T2", "D2", nil, nil, "done", true, int64(3), int64(2)),
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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

	userID := int64(7)

	countQuery := regexp.QuoteMeta(`
		SELECT COUNT(*) FROM tasks WHERE user_id = $1
	`)
	mock.ExpectQuery(countQuery).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	dataQuery := regexp.QuoteMeta(`
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken
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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

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

	s := &Storage{DB: db}

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

func TestGetUserStats_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(123)
	now := time.Now().UTC().Truncate(time.Second)
	points := int64(100)
	level := int64(2)
	totalPomodoros := int64(10)
	totalBurntTasks := int64(3)
	currentStreak := int64(5)
	bestStreak := int64(7)

	query := regexp.QuoteMeta(`
       SELECT id, user_id, points, level, total_pomodoros, total_burnt_tasks, current_streak, best_streak, updated_at FROM user_stats WHERE user_id = $1
       `)
	mock.ExpectQuery(query).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "points", "level", "total_pomodoros", "total_burnt_tasks", "current_streak", "best_streak", "updated_at"}).
			AddRow(int64(1), userID, points, level, totalPomodoros, totalBurntTasks, currentStreak, bestStreak, now))

	stats, err := s.GetUserStats(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, userID, stats.UserID)
	require.NotNil(t, stats.Points)
	require.Equal(t, points, *stats.Points)
	require.NotNil(t, stats.Level)
	require.Equal(t, level, *stats.Level)
	require.NotNil(t, stats.TotalPomodoros)
	require.Equal(t, totalPomodoros, *stats.TotalPomodoros)
	require.NotNil(t, stats.TotalBurntTasks)
	require.Equal(t, totalBurntTasks, *stats.TotalBurntTasks)
	require.NotNil(t, stats.CurrentStreak)
	require.Equal(t, currentStreak, *stats.CurrentStreak)
	require.NotNil(t, stats.BestStreak)
	require.Equal(t, bestStreak, *stats.BestStreak)
	require.NotNil(t, stats.UpdatedAt)
	require.WithinDuration(t, now, *stats.UpdatedAt, time.Second)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserStats_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(123)

	query := regexp.QuoteMeta(`
       SELECT id, user_id, points, level, total_pomodoros, total_burnt_tasks, current_streak, best_streak, updated_at FROM user_stats WHERE user_id = $1
       `)
	mock.ExpectQuery(query).
		WithArgs(userID).
		WillReturnError(errors.New("some generated error"))

	stats, err := s.GetUserStats(context.Background(), userID)
	require.Error(t, err)
	require.Equal(t, int64(0), stats.ID)
	require.Contains(t, err.Error(), "some generated error")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUserStats_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(123)
	points := int64(100)
	level := int64(2)
	totalPomodoros := int64(10)
	totalBurntTasks := int64(3)
	currentStreak := int64(5)
	bestStreak := int64(7)

	query := regexp.QuoteMeta(`
	INSERT INTO user_stats (
    user_id, 
    points, 
    level, 
    total_pomodoros, 
    total_burnt_tasks, 
    current_streak, 
    best_streak, 
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (user_id) DO UPDATE SET
    points          = COALESCE(EXCLUDED.points, user_stats.points),
    level           = COALESCE(EXCLUDED.level, user_stats.level),
    total_pomodoros = COALESCE(EXCLUDED.total_pomodoros, user_stats.total_pomodoros),
    total_burnt_tasks = COALESCE(EXCLUDED.total_burnt_tasks, user_stats.total_burnt_tasks),
    current_streak  = COALESCE(EXCLUDED.current_streak, user_stats.current_streak),
    best_streak     = COALESCE(EXCLUDED.best_streak, user_stats.best_streak),
    updated_at      = NOW()
;
       `)
	mock.ExpectExec(query).
		WithArgs(userID, &points, &level, &totalPomodoros, &totalBurntTasks, &currentStreak, &bestStreak, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	stats := domain.UserStatsUpdate{
		Points:          &points,
		Level:           &level,
		TotalPomodoros:  &totalPomodoros,
		TotalBurntTasks: &totalBurntTasks,
		CurrentStreak:   &currentStreak,
		BestStreak:      &bestStreak,
	}

	err = s.UpdateUserStats(context.Background(), userID, stats)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUserStats_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(123)
	points := int64(100)
	level := int64(2)
	totalPomodoros := int64(10)
	totalBurntTasks := int64(3)
	currentStreak := int64(5)
	bestStreak := int64(7)

	query := regexp.QuoteMeta(`
	INSERT INTO user_stats (
    user_id, 
    points, 
    level, 
    total_pomodoros, 
    total_burnt_tasks, 
    current_streak, 
    best_streak, 
    updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (user_id) DO UPDATE SET
    points          = COALESCE(EXCLUDED.points, user_stats.points),
    level           = COALESCE(EXCLUDED.level, user_stats.level),
    total_pomodoros = COALESCE(EXCLUDED.total_pomodoros, user_stats.total_pomodoros),
    total_burnt_tasks = COALESCE(EXCLUDED.total_burnt_tasks, user_stats.total_burnt_tasks),
    current_streak  = COALESCE(EXCLUDED.current_streak, user_stats.current_streak),
    best_streak     = COALESCE(EXCLUDED.best_streak, user_stats.best_streak),
    updated_at      = NOW()
;
       `)
	mock.ExpectExec(query).
		WithArgs(userID, &points, &level, &totalPomodoros, &totalBurntTasks, &currentStreak, &bestStreak, sqlmock.AnyArg()).
		WillReturnError(errors.New("update failed"))

	stats := domain.UserStatsUpdate{
		Points:          &points,
		Level:           &level,
		TotalPomodoros:  &totalPomodoros,
		TotalBurntTasks: &totalBurntTasks,
		CurrentStreak:   &currentStreak,
		BestStreak:      &bestStreak,
	}

	err = s.UpdateUserStats(context.Background(), userID, stats)
	require.Error(t, err)
	require.Contains(t, err.Error(), "update failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for SaveUser
func TestSaveUser_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	email := "test@example.com"
	passHash := []byte("hashed_password")

	query := regexp.QuoteMeta(`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING ID`)
	mock.ExpectQuery(query).
		WithArgs(email, passHash).
		WillReturnRows(sqlmock.NewRows([]string{"ID"}).AddRow(int64(42)))

	id, err := s.SaveUser(context.Background(), email, passHash)
	require.NoError(t, err)
	require.Equal(t, int64(42), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveUser_AlreadyExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	email := "test@example.com"
	passHash := []byte("hashed_password")

	query := regexp.QuoteMeta(`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING ID`)
	mock.ExpectQuery(query).
		WithArgs(email, passHash).
		WillReturnError(&pgconn.PgError{Code: pgerrcode.UniqueViolation})

	_, err = s.SaveUser(context.Background(), email, passHash)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrUserExists)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for User (by email)
func TestUser_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	email := "test@example.com"
	passHash := []byte("hashed_password")

	query := regexp.QuoteMeta(`SELECT id, email, password_hash, is_verified FROM users WHERE email = $1`)
	mock.ExpectQuery(query).
		WithArgs(email).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified"}).
			AddRow(int64(42), email, passHash, false))

	user, err := s.User(context.Background(), email)
	require.NoError(t, err)
	require.Equal(t, int64(42), user.ID)
	require.Equal(t, email, user.Email)
	require.Equal(t, passHash, user.PassHash)
	require.False(t, user.IsVerified)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUser_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	email := "notfound@example.com"

	query := regexp.QuoteMeta(`SELECT id, email, password_hash, is_verified FROM users WHERE email = $1`)
	mock.ExpectQuery(query).
		WithArgs(email).
		WillReturnError(sql.ErrNoRows)

	_, err = s.User(context.Background(), email)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrUserNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for GetUserByID
func TestGetUserByID_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	email := "user@example.com"
	passHash := []byte("hashed_password")

	query := regexp.QuoteMeta(`
	SELECT id, email, password_hash, is_verified FROM users
	WHERE id = $1
	`)
	mock.ExpectQuery(query).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified"}).
			AddRow(userID, email, passHash, true))

	user, err := s.GetUserByID(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, userID, user.ID)
	require.Equal(t, email, user.Email)
	require.True(t, user.IsVerified)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(999)

	query := regexp.QuoteMeta(`
	SELECT id, email, password_hash, is_verified FROM users
	WHERE id = $1
	`)
	mock.ExpectQuery(query).
		WithArgs(userID).
		WillReturnError(sql.ErrNoRows)

	user, err := s.GetUserByID(context.Background(), userID)
	require.Error(t, err)
	require.Nil(t, user)
	require.ErrorIs(t, err, storage.ErrUserNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for SaveTask
func TestSaveTask_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	task := domain.Task{
		UserID:      int64(42),
		Title:       "Test Task",
		Description: "Testing",
		Status:      "pending",
		IsNotified:  false,
	}

	query := regexp.QuoteMeta(`
INSERT INTO tasks (user_id, title, description, deadline, reminder_at, status, is_notified, category_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id`)
	mock.ExpectQuery(query).
		WithArgs(task.UserID, task.Title, task.Description, nil, nil, task.Status, task.IsNotified, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))

	id, err := s.SaveTask(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, int64(10), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveTask_WithCategory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	categoryID := int64(5)
	task := domain.Task{
		UserID:      int64(42),
		Title:       "Test Task",
		Description: "Testing",
		Status:      "pending",
		IsNotified:  false,
		CategoryID:  &categoryID,
	}

	// Check if category exists
	existsQuery := regexp.QuoteMeta(`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)`)
	mock.ExpectQuery(existsQuery).
		WithArgs(categoryID, task.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Insert task
	query := regexp.QuoteMeta(`
INSERT INTO tasks (user_id, title, description, deadline, reminder_at, status, is_notified, category_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id`)
	mock.ExpectQuery(query).
		WithArgs(task.UserID, task.Title, task.Description, nil, nil, task.Status, task.IsNotified, categoryID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))

	id, err := s.SaveTask(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, int64(10), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveTask_CategoryNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	categoryID := int64(5)
	task := domain.Task{
		UserID:     int64(42),
		Title:      "Test Task",
		CategoryID: &categoryID,
	}

	// Check if category exists - returns false
	existsQuery := regexp.QuoteMeta(`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)`)
	mock.ExpectQuery(existsQuery).
		WithArgs(categoryID, task.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	_, err = s.SaveTask(context.Background(), task)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrCategoryNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for UpdateTask
func TestUpdateTask_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(10)
	newTitle := "Updated Title"
	status := "done"

	query := regexp.QuoteMeta(`
			UPDATE tasks
			SET title = $1, status = $2
			WHERE user_id = $3 AND id = $4
`)
	mock.ExpectExec(query).
		WithArgs(newTitle, status, userID, taskID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.UpdateTask(context.Background(), userID, taskID, domain.TaskUpdate{
		Title:  &newTitle,
		Status: &status,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTask_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(999)
	newTitle := "Updated Title"

	query := regexp.QuoteMeta(`
			UPDATE tasks
			SET title = $1
			WHERE user_id = $2 AND id = $3
`)
	mock.ExpectExec(query).
		WithArgs(newTitle, userID, taskID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = s.UpdateTask(context.Background(), userID, taskID, domain.TaskUpdate{
		Title: &newTitle,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrTaskNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for DeleteTask
func TestDeleteTask_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(10)

	query := regexp.QuoteMeta(`
		DELETE FROM tasks 
		       WHERE user_id=$1 AND id=$2
		RETURNING id
`)
	mock.ExpectQuery(query).
		WithArgs(userID, taskID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(taskID))

	err = s.DeleteTask(context.Background(), userID, taskID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTask_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(999)

	query := regexp.QuoteMeta(`
		DELETE FROM tasks 
		       WHERE user_id=$1 AND id=$2
		RETURNING id
`)
	mock.ExpectQuery(query).
		WithArgs(userID, taskID).
		WillReturnError(sql.ErrNoRows)

	err = s.DeleteTask(context.Background(), userID, taskID)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrTaskNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for Refresh Tokens
func TestSaveRefreshToken_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	token := "refresh_token_value"
	expiresAt := time.Now().Add(24 * time.Hour)

	query := regexp.QuoteMeta(`
		INSERT INTO refresh_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`)
	mock.ExpectExec(query).
		WithArgs(userID, token, expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = s.SaveRefreshToken(context.Background(), userID, token, expiresAt)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRefreshToken_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	token := "refresh_token_value"
	userID := int64(42)
	expiresAt := time.Now().Add(24 * time.Hour)

	query := regexp.QuoteMeta(`
	SELECT id, user_id, token, expires_at, created_at
	 FROM refresh_tokens
	 WHERE token = $1 AND expires_at > NOW()
	 `)
	mock.ExpectQuery(query).
		WithArgs(token).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at", "created_at"}).
			AddRow(int64(1), userID, token, expiresAt, time.Now()))

	rt, err := s.GetRefreshToken(context.Background(), token)
	require.NoError(t, err)
	require.NotNil(t, rt)
	require.Equal(t, userID, rt.UserID)
	require.Equal(t, token, rt.Token)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRefreshToken_Invalid(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	token := "invalid_token"

	query := regexp.QuoteMeta(`
	SELECT id, user_id, token, expires_at, created_at
	 FROM refresh_tokens
	 WHERE token = $1 AND expires_at > NOW()
	 `)
	mock.ExpectQuery(query).
		WithArgs(token).
		WillReturnError(sql.ErrNoRows)

	rt, err := s.GetRefreshToken(context.Background(), token)
	require.Error(t, err)
	require.Nil(t, rt)
	require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteRefreshToken_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	token := "refresh_token_value"

	query := regexp.QuoteMeta(`
	DELETE FROM refresh_tokens
	WHERE token = $1
`)
	mock.ExpectExec(query).
		WithArgs(token).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.DeleteRefreshToken(context.Background(), token)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteRefreshToken_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	token := "nonexistent_token"

	query := regexp.QuoteMeta(`
	DELETE FROM refresh_tokens
	WHERE token = $1
`)
	mock.ExpectExec(query).
		WithArgs(token).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = s.DeleteRefreshToken(context.Background(), token)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Tests for Email Verification Tokens
func TestSaveEmailVerificationToken_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	token := "verification_token"
	expiresAt := time.Now().Add(24 * time.Hour)

	query := regexp.QuoteMeta(`
		INSERT INTO email_verification_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`)
	mock.ExpectExec(query).
		WithArgs(userID, token, expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = s.SaveEmailVerificationToken(context.Background(), userID, token, expiresAt)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEmailVerificationToken_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	token := "verification_token"

	query := regexp.QuoteMeta(`
		SELECT id, user_id, token, expires_at, created_at 
		FROM email_verification_tokens 
		WHERE token = $1
	`)
	mock.ExpectQuery(query).
		WithArgs(token).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at", "created_at"}).
			AddRow(int64(1), userID, token, time.Now().Add(24*time.Hour), time.Now()))

	evt, err := s.GetEmailVerificationToken(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, userID, evt.UserID)
	require.Equal(t, token, evt.Token)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEmailVerificationToken_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	token := "invalid_token"

	query := regexp.QuoteMeta(`
		SELECT id, user_id, token, expires_at, created_at 
		FROM email_verification_tokens 
		WHERE token = $1
	`)
	mock.ExpectQuery(query).
		WithArgs(token).
		WillReturnError(sql.ErrNoRows)

	_, err = s.GetEmailVerificationToken(context.Background(), token)
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrTokenNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyUserEmail_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)

	query := regexp.QuoteMeta(`
		UPDATE users SET is_verified = true WHERE id = $1
	`)
	mock.ExpectExec(query).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.VerifyUserEmail(context.Background(), userID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
