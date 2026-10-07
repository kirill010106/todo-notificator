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
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken, reward_claimed
FROM tasks
	WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`)

	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(dataQuery).
		WithArgs(userID, limit, offset).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "user_id", "title", "description", "deadline", "reminder_at", "status", "is_notified", "category_id", "pomodoro_taken", "reward_claimed"}).
				AddRow(int64(10), userID, "T1", "D1", now, now, "pending", false, int64(0), int64(0), false).
				AddRow(int64(9), userID, "T2", "D2", nil, nil, "done", true, int64(3), int64(2), true),
		)

	tasks, total, err := s.GetTasks(context.Background(), userID, limit, offset, domain.TaskFilter{})
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

	tasks, total, err := s.GetTasks(context.Background(), 1, 10, 0, domain.TaskFilter{})
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
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken, reward_claimed
FROM tasks
	WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`)
	mock.ExpectQuery(dataQuery).
		WithArgs(userID, 10, 0).
		WillReturnError(errors.New("data failed"))

	tasks, total, err := s.GetTasks(context.Background(), userID, 10, 0, domain.TaskFilter{})
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

	query := regexp.QuoteMeta(`SELECT id, email, password_hash, is_verified, is_premium FROM users WHERE email = $1`)
	mock.ExpectQuery(query).
		WithArgs(email).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified", "is_premium"}).
			AddRow(int64(42), email, passHash, false, true))

	user, err := s.User(context.Background(), email)
	require.NoError(t, err)
	require.Equal(t, int64(42), user.ID)
	require.Equal(t, email, user.Email)
	require.Equal(t, passHash, user.PassHash)
	require.False(t, user.IsVerified)
	require.True(t, user.IsPremium)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUser_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	email := "notfound@example.com"

	query := regexp.QuoteMeta(`SELECT id, email, password_hash, is_verified, is_premium FROM users WHERE email = $1`)
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
	SELECT id, email, password_hash, is_verified, is_premium FROM users
	WHERE id = $1
	`)
	mock.ExpectQuery(query).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified", "is_premium"}).
			AddRow(userID, email, passHash, true, true))

	user, err := s.GetUserByID(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, userID, user.ID)
	require.Equal(t, email, user.Email)
	require.True(t, user.IsVerified)
	require.True(t, user.IsPremium)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUserByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(999)

	query := regexp.QuoteMeta(`
	SELECT id, email, password_hash, is_verified, is_premium FROM users
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

func TestUpdateTask_ReminderAtResetsNotificationFlag(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(10)
	reminderAt := time.Now().Add(30 * time.Minute).UTC()

	query := regexp.QuoteMeta(`
			UPDATE tasks
			SET reminder_at = $1, is_notified = false
			WHERE user_id = $2 AND id = $3
`)
	mock.ExpectExec(query).
		WithArgs(reminderAt, userID, taskID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.UpdateTask(context.Background(), userID, taskID, domain.TaskUpdate{
		ReminderAt: &reminderAt,
	})
	require.NoError(t, err)
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

func TestRotateRefreshToken_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}

	oldToken := "old_refresh_token"
	newToken := "new_refresh_token"
	expiresAt := time.Now().Add(24 * time.Hour)
	passHash := []byte("hashed_password")

	query := regexp.QuoteMeta(`
		WITH rotated AS (
			UPDATE refresh_tokens
			SET token = $2, expires_at = $3
			WHERE token = $1 AND expires_at > NOW()
			RETURNING user_id
		)
		SELECT u.id, u.email, u.password_hash, u.is_verified, u.is_premium
		FROM rotated r
		JOIN users u ON u.id = r.user_id
	`)

	mock.ExpectQuery(query).
		WithArgs(oldToken, newToken, expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified", "is_premium"}).
			AddRow(int64(42), "user@example.com", passHash, true, true))

	user, err := s.RotateRefreshToken(context.Background(), oldToken, newToken, expiresAt)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, int64(42), user.ID)
	require.Equal(t, "user@example.com", user.Email)
	require.Equal(t, passHash, user.PassHash)
	require.True(t, user.IsVerified)
	require.True(t, user.IsPremium)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRotateRefreshToken_Invalid(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}

	oldToken := "old_refresh_token"
	newToken := "new_refresh_token"
	expiresAt := time.Now().Add(24 * time.Hour)

	query := regexp.QuoteMeta(`
		WITH rotated AS (
			UPDATE refresh_tokens
			SET token = $2, expires_at = $3
			WHERE token = $1 AND expires_at > NOW()
			RETURNING user_id
		)
		SELECT u.id, u.email, u.password_hash, u.is_verified, u.is_premium
		FROM rotated r
		JOIN users u ON u.id = r.user_id
	`)

	mock.ExpectQuery(query).
		WithArgs(oldToken, newToken, expiresAt).
		WillReturnError(sql.ErrNoRows)

	user, err := s.RotateRefreshToken(context.Background(), oldToken, newToken, expiresAt)
	require.Error(t, err)
	require.Nil(t, user)
	require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRotateRefreshToken_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}

	oldToken := "old_refresh_token"
	newToken := "new_refresh_token"
	expiresAt := time.Now().Add(24 * time.Hour)

	query := regexp.QuoteMeta(`
		WITH rotated AS (
			UPDATE refresh_tokens
			SET token = $2, expires_at = $3
			WHERE token = $1 AND expires_at > NOW()
			RETURNING user_id
		)
		SELECT u.id, u.email, u.password_hash, u.is_verified, u.is_premium
		FROM rotated r
		JOIN users u ON u.id = r.user_id
	`)

	mock.ExpectQuery(query).
		WithArgs(oldToken, newToken, expiresAt).
		WillReturnError(errors.New("db error"))

	user, err := s.RotateRefreshToken(context.Background(), oldToken, newToken, expiresAt)
	require.Error(t, err)
	require.Nil(t, user)
	require.Contains(t, err.Error(), "db error")
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

func TestStorage_GetTask(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(1)
	taskID := int64(1)

	now := time.Now()
	description := "Some description"
	expectedTask := domain.Task{
		ID:            taskID,
		Title:         "Test Task",
		Description:   description,
		UserID:        userID,
		Status:        domain.TaskStatusPending,
		ReminderAt:    &now,
		RewardClaimed: true,
	}

	t.Run("success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "title", "description", "deadline", "reminder_at", "status", "is_notified", "category_id", "pomodoro_taken", "reward_claimed"}).
			AddRow(
				expectedTask.ID, expectedTask.UserID, expectedTask.Title, expectedTask.Description, expectedTask.Deadline, expectedTask.ReminderAt, expectedTask.Status, false, expectedTask.CategoryID, expectedTask.PomodorosTaken, expectedTask.RewardClaimed,
			)

		mock.ExpectQuery("SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken, reward_claimed FROM tasks WHERE user_id = \\$1 AND id = \\$2").
			WithArgs(userID, taskID).
			WillReturnRows(rows)

		task, err := s.GetTask(context.Background(), userID, taskID)

		require.NoError(t, err)
		require.Equal(t, expectedTask.ID, task.ID)
		require.Equal(t, expectedTask.Title, task.Title)
		require.Equal(t, expectedTask.RewardClaimed, task.RewardClaimed)
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken, reward_claimed FROM tasks WHERE user_id = \\$1 AND id = \\$2").
			WithArgs(userID, taskID).
			WillReturnError(sql.ErrNoRows)

		_, err := s.GetTask(context.Background(), userID, taskID)

		require.ErrorIs(t, err, storage.ErrTaskNotFound)
	})
}

func TestCountCategories(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(10)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM categories WHERE user_id = $1")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := s.CountCategories(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCountActiveReminders(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(10)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tasks WHERE user_id = $1 AND reminder_at IS NOT NULL AND status != 'done'")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := s.CountActiveReminders(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetPremiumStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(10)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE users SET is_premium = $2 WHERE id = $1")).
		WithArgs(userID, true).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = s.SetPremiumStatus(context.Background(), userID, true)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLatestPendingPayment(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(10)

	query := regexp.QuoteMeta(`
		SELECT yookassa_payment_id FROM payments
		WHERE user_id = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT 1
	`)

	mock.ExpectQuery(query).
		WithArgs(userID, domain.PaymentStatusPending).
		WillReturnRows(sqlmock.NewRows([]string{"yookassa_payment_id"}).AddRow("yoo-123"))

	paymentID, err := s.GetLatestPendingPayment(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "yoo-123", paymentID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkDeleteTasks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(1)

	// Empty list returns 0, nil without DB call
	deleted, err := s.BulkDeleteTasks(context.Background(), userID, nil)
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)

	taskIDs := []int64{10, 20}
	query := regexp.QuoteMeta(`DELETE FROM tasks WHERE user_id = $1 AND id IN ($2, $3)`)
	mock.ExpectExec(query).
		WithArgs(userID, int64(10), int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	deleted, err = s.BulkDeleteTasks(context.Background(), userID, taskIDs)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkCompleteTasks(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(1)

	// Empty list returns 0, nil without DB call
	completed, err := s.BulkCompleteTasks(context.Background(), userID, nil)
	require.NoError(t, err)
	require.Equal(t, int64(0), completed)

	taskIDs := []int64{10, 20}
	query := regexp.QuoteMeta(`UPDATE tasks SET status = $1, reminder_at = NULL WHERE user_id = $2 AND id IN ($3, $4) AND status != $1`)
	mock.ExpectExec(query).
		WithArgs(domain.TaskStatusDone, userID, int64(10), int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	completed, err = s.BulkCompleteTasks(context.Background(), userID, taskIDs)
	require.NoError(t, err)
	require.Equal(t, int64(2), completed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUnlockAchievement(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(1)

	// Case 1: First time unlocked with points award
	insertQuery := regexp.QuoteMeta(`INSERT INTO achievements (user_id, code) VALUES ($1, $2) ON CONFLICT (user_id, code) DO NOTHING RETURNING id`)
	mock.ExpectQuery(insertQuery).
		WithArgs(userID, "first_step").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(100)))

	ensureStatsQuery := regexp.QuoteMeta(`INSERT INTO user_stats (user_id) VALUES ($1) ON CONFLICT DO NOTHING`)
	mock.ExpectExec(ensureStatsQuery).
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	statsQuery := regexp.QuoteMeta(`UPDATE user_stats`)
	mock.ExpectQuery(statsQuery).
		WithArgs(25, 0, 0, false, false, userID).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow(int64(25)))

	levelQuery := regexp.QuoteMeta(`UPDATE user_stats SET level = $1 WHERE user_id = $2`)
	mock.ExpectExec(levelQuery).
		WithArgs(1, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	unlocked, err := s.UnlockAchievement(context.Background(), userID, "first_step")
	require.NoError(t, err)
	require.True(t, unlocked)

	// Case 2: Already unlocked (ON CONFLICT DO NOTHING returns no rows)
	mock.ExpectQuery(insertQuery).
		WithArgs(userID, "first_step").
		WillReturnError(sql.ErrNoRows)

	unlocked, err = s.UnlockAchievement(context.Background(), userID, "first_step")
	require.NoError(t, err)
	require.False(t, unlocked)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDailyQuests(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(1)

	q1 := regexp.QuoteMeta(`SELECT COUNT(*) FROM tasks WHERE user_id = $1 AND created_at >= CURRENT_DATE`)
	mock.ExpectQuery(q1).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	q2 := regexp.QuoteMeta(`SELECT COUNT(*) FROM pomodoro_sessions WHERE user_id = $1 AND completed_at >= CURRENT_DATE`)
	mock.ExpectQuery(q2).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	q3 := regexp.QuoteMeta(`SELECT COUNT(*) FROM tasks WHERE user_id = $1 AND status = 'done' AND updated_at >= CURRENT_DATE`)
	mock.ExpectQuery(q3).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	quests, err := s.GetDailyQuests(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, quests, 3)
	require.True(t, quests[0].Completed) // 2 >= 1
	require.True(t, quests[1].Completed) // 1 >= 1
	require.True(t, quests[2].Completed) // 3 >= 3
	require.NoError(t, mock.ExpectationsWereMet())
}



