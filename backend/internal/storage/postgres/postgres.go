package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type Storage struct {
	DB *sql.DB
}

func New(storagePath string) (*Storage, error) {
	const op = "storage.postgres.new"
	db, err := sql.Open("pgx", storagePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &Storage{DB: db}, nil
}

func (s *Storage) Close() error {
	if s.DB != nil {
		return s.DB.Close()
	}
	return nil
}

func (s *Storage) saveTaskRow(ctx context.Context, task domain.Task) (int64, error) {
	const op = "storage.postgres.saveTask"

	var categoryArg any = nil
	if task.CategoryID != nil && *task.CategoryID > 0 {
		var exists bool
		err := s.executor(ctx).QueryRowContext(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)`,
			task.CategoryID, task.UserID,
		).Scan(&exists)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", op, err)
		}
		if !exists {
			return 0, fmt.Errorf("%s: %w", op, storage.ErrCategoryNotFound)
		}
		categoryArg = task.CategoryID
	}

	query := `
INSERT INTO tasks (user_id, title, description, deadline, status, category_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id`
	var id int64

	err := s.executor(ctx).QueryRowContext(ctx, query, task.UserID, task.Title, task.Description, task.Deadline, task.Status, categoryArg).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return 0, fmt.Errorf("%s: %w", op, storage.ErrTaskExists)
			}
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if outcome := actionOutcome(ctx); outcome != nil {
		outcome.Result.ChangedTaskIDs = append(outcome.Result.ChangedTaskIDs, id)
	}
	return id, nil
}

func (s *Storage) GetTask(ctx context.Context, userID int64, taskID int64) (domain.Task, error) {
	const op = "storage.postgres.GetTask"

	query := `
		SELECT id, user_id, title, description, deadline, ` + reminderJSON + `, status, category_id, pomodoro_taken, reward_claimed
		FROM tasks
		WHERE user_id = $1 AND id = $2
	`
	var t domain.Task
	var reminders []byte
	err := s.executor(ctx).QueryRowContext(ctx, query, userID, taskID).Scan(
		&t.ID,
		&t.UserID,
		&t.Title,
		&t.Description,
		&t.Deadline,
		&reminders,
		&t.Status,
		&t.CategoryID,
		&t.PomodorosTaken,
		&t.RewardClaimed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, fmt.Errorf("%s: %w", op, storage.ErrTaskNotFound)
		}
		return domain.Task{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := decodeReminders(reminders, &t); err != nil {
		return domain.Task{}, err
	}
	return t, nil
}

func (s *Storage) GetTasks(ctx context.Context, userID int64, limit, offset int, filter domain.TaskFilter) ([]domain.Task, int, error) {
	const op = "storage.postgres.GetTasks"

	// Build dynamic WHERE conditions
	conditions := []string{"user_id = $1"}
	args := []any{userID}
	argID := 2

	if filter.Status != nil && *filter.Status != "all" && *filter.Status != "" {
		if domain.ValidTaskStatuses[*filter.Status] {
			conditions = append(conditions, fmt.Sprintf("status = $%d", argID))
			args = append(args, *filter.Status)
			argID++
		}
	}

	if filter.CategoryID != nil {
		conditions = append(conditions, fmt.Sprintf("category_id = $%d", argID))
		args = append(args, *filter.CategoryID)
		argID++
	}

	if filter.Search != nil && *filter.Search != "" {
		searchPattern := "%" + *filter.Search + "%"
		conditions = append(conditions, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", argID, argID))
		args = append(args, searchPattern)
		argID++
	}

	whereClause := strings.Join(conditions, " AND ")
	if filter.DeadlineFrom != nil {
		conditions = append(conditions, fmt.Sprintf("deadline >= $%d", argID))
		args = append(args, *filter.DeadlineFrom)
		argID++
	}
	if filter.DeadlineBefore != nil {
		conditions = append(conditions, fmt.Sprintf("deadline < $%d", argID))
		args = append(args, *filter.DeadlineBefore)
		argID++
	}
	if filter.HasDeadline != nil {
		if *filter.HasDeadline {
			conditions = append(conditions, "deadline IS NOT NULL")
		} else {
			conditions = append(conditions, "deadline IS NULL")
		}
	}
	whereClause = strings.Join(conditions, " AND ")

	// Count query
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM tasks WHERE %s`, whereClause) //nolint:gosec

	var total int
	if err := s.executor(ctx).QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	// Dynamic sorting
	orderBy := "created_at DESC, id DESC"
	if filter.SortBy != nil {
		switch *filter.SortBy {
		case "deadline":
			dir := "ASC NULLS LAST"
			if filter.Order != nil && strings.ToLower(*filter.Order) == "desc" {
				dir = "DESC NULLS LAST"
			}
			orderBy = fmt.Sprintf("deadline %s, id DESC", dir)
		case "created_at":
			dir := "DESC"
			if filter.Order != nil && strings.ToLower(*filter.Order) == "asc" {
				dir = "ASC"
			}
			orderBy = fmt.Sprintf("created_at %s, id DESC", dir)
		}
	}

	// Data query
	dataArgs := make([]any, len(args))
	copy(dataArgs, args)
	dataArgs = append(dataArgs, limit, offset)

	dataQuery := fmt.Sprintf(`
SELECT id, user_id, title, description, deadline, `+reminderJSON+`, status, category_id, pomodoro_taken, reward_claimed
FROM tasks
	WHERE %s ORDER BY %s
LIMIT $%d OFFSET $%d`, whereClause, orderBy, argID, argID+1) //nolint:gosec

	rows, err := s.executor(ctx).QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close() //nolint:errcheck

	tasks := make([]domain.Task, 0, limit)

	for rows.Next() {
		var t domain.Task
		var reminders []byte
		err = rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Deadline, &reminders, &t.Status, &t.CategoryID, &t.PomodorosTaken, &t.RewardClaimed)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", op, err)
		}
		if err := decodeReminders(reminders, &t); err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, t)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, total, nil
}

func (s *Storage) BulkDeleteTasks(ctx context.Context, userID int64, taskIDs []int64) (int64, error) {
	const op = "storage.postgres.BulkDeleteTasks"
	if len(taskIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(taskIDs))
	args := make([]any, 0, len(taskIDs)+1)
	args = append(args, userID)
	for i, id := range taskIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	query := fmt.Sprintf(`DELETE FROM tasks WHERE user_id = $1 AND id IN (%s)`, strings.Join(placeholders, ", "))
	res, err := s.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return rowsAffected, nil
}

func (s *Storage) BulkCompleteTasks(ctx context.Context, userID int64, taskIDs []int64) (int64, error) {
	return s.bulkCompleteWithRewards(ctx, userID, taskIDs)
}

func (s *Storage) DeleteTask(ctx context.Context, userID int64, taskID int64) error {
	const op = "storage.postgres.DeleteTask"

	query := `
		DELETE FROM tasks 
		       WHERE user_id=$1 AND id=$2
		RETURNING id
`
	var deletedID int64
	err := s.DB.QueryRowContext(ctx, query, userID, taskID).Scan(&deletedID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s: %w", op, storage.ErrTaskNotFound)
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *Storage) UpdateTask(ctx context.Context, userID int64, taskID int64, t domain.TaskUpdate) error {
	return s.updateTaskWithRewards(ctx, userID, taskID, t)
}

func updateTaskSQL(ctx context.Context, db sqlExecutor, userID int64, taskID int64, t domain.TaskUpdate) error {
	const op = "storage.postgres.UpdateTask"

	setValues := make([]string, 0)
	args := make([]any, 0)
	argID := 1

	if t.Title != nil {
		setValues = append(setValues, fmt.Sprintf("title = $%d", argID))
		args = append(args, *t.Title)
		argID++
	}

	if t.Description != nil {
		setValues = append(setValues, fmt.Sprintf("description = $%d", argID))
		args = append(args, *t.Description)
		argID++
	}

	if t.Status != nil {
		setValues = append(setValues, fmt.Sprintf("status = $%d", argID))
		args = append(args, *t.Status)
		argID++
	}

	if t.Deadline != nil {
		setValues = append(setValues, fmt.Sprintf("deadline = $%d", argID))
		args = append(args, *t.Deadline)
		argID++
	}
	if t.ClearDeadline {
		setValues = append(setValues, "deadline = NULL")
	}
	if t.ClearCategoryID {
		setValues = append(setValues, "category_id = NULL")
	}

	if t.CategoryID != nil {
		var exists bool
		err := db.QueryRowContext(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)`,
			*t.CategoryID, userID,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if !exists {
			return storage.ErrCategoryNotFound
		}

		setValues = append(setValues, fmt.Sprintf("category_id = $%d", argID))
		args = append(args, *t.CategoryID)
		argID++
	}

	if t.IncrementPomodorosTaken {
		setValues = append(setValues, "pomodoro_taken = pomodoro_taken + 1")
	}

	if t.RewardClaimed {
		setValues = append(setValues, "reward_claimed = true")
	}
	if len(setValues) > 0 {
		setValues = append(setValues, "updated_at = CURRENT_TIMESTAMP")
	} else {
		return nil
	}

	if len(setValues) == 0 {
		return nil
	}

	query := fmt.Sprintf(`UPDATE tasks SET %s WHERE user_id = $%d AND id = $%d`, strings.Join(setValues, ", "), argID, argID+1) //nolint:gosec

	args = append(args, userID, taskID)

	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return storage.ErrTaskNotFound
	}

	return nil
}

func (s *Storage) User(ctx context.Context, email string) (domain.User, error) {
	const op = "storage.postgres.User"

	query := `SELECT id, email, password_hash, is_verified, is_premium FROM users WHERE email = $1`

	var user domain.User
	err := s.DB.QueryRowContext(ctx, query, email).Scan(&user.ID, &user.Email, &user.PassHash, &user.IsVerified, &user.IsPremium)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, storage.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("%s: %w", op, err)
	}
	return user, nil
}

func (s *Storage) SaveUser(ctx context.Context, email string, passHash []byte) (int64, error) {
	const op = "storage.postgres.SaveUser"

	query := `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING ID`

	var id int64
	err := s.DB.QueryRowContext(ctx, query, email, passHash).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return 0, storage.ErrUserExists
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return id, nil
}

// SaveRefreshToken is DEPRECATED: WILL BE DELETED SOON, REPLACED BY RotateRefreshToken due to security problems
func (s *Storage) SaveRefreshToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	const op = "storage.postgres.SaveRefreshToken"

	query := `
		INSERT INTO refresh_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`

	// refresh_tokens.expires_at is a legacy timestamp without time zone.
	// Persist UTC wall time and compare it with an explicitly UTC database clock.
	_, err := s.DB.ExecContext(ctx, query, userID, token, expiresAt.UTC())
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetRefreshToken is DEPRECATED: WILL BE DELETED SOON, REPLACED BY RotateRefreshToken due to security problems
func (s *Storage) GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	const op = "storage.postgres.GetRefreshToken"

	query := `
	SELECT id, user_id, token, expires_at, created_at
	 FROM refresh_tokens
	 WHERE token = $1 AND expires_at > (NOW() AT TIME ZONE 'UTC')
	 `
	var rt domain.RefreshToken
	err := s.DB.QueryRowContext(ctx, query, token).Scan(
		&rt.ID,
		&rt.UserID,
		&rt.Token,
		&rt.ExpiresAt,
		&rt.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, storage.ErrRefreshTokenInvalid)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &rt, nil
}

// DeleteRefreshToken is DEPRECATED: WILL BE DELETED SOON, REPLACED BY RotateRefreshToken due to security problems
func (s *Storage) DeleteRefreshToken(ctx context.Context, token string) error {
	const op = "storage.postgres.DeleteRefreshToken"

	query := `
	DELETE FROM refresh_tokens
	WHERE token = $1
`
	res, err := s.DB.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, storage.ErrRefreshTokenInvalid)
	}

	return nil
}

func (s *Storage) RotateRefreshToken(ctx context.Context, oldToken string, newToken string, expiresAt time.Time) (*domain.User, error) {
	const op = "storage.postgres.RotateRefreshToken"

	query := `
		WITH rotated AS (
			UPDATE refresh_tokens
			SET token = $2, expires_at = $3
			WHERE token = $1 AND expires_at > (NOW() AT TIME ZONE 'UTC')
			RETURNING user_id
		)
		SELECT u.id, u.email, u.password_hash, u.is_verified, u.is_premium
		FROM rotated r
		JOIN users u ON u.id = r.user_id
	`

	var user domain.User
	err := s.DB.QueryRowContext(ctx, query, oldToken, newToken, expiresAt.UTC()).Scan(
		&user.ID,
		&user.Email,
		&user.PassHash,
		&user.IsVerified,
		&user.IsPremium,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, storage.ErrRefreshTokenInvalid)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &user, nil
}

func (s *Storage) DeleteUserRefreshTokens(ctx context.Context, userID int64) error {
	const op = "storage.postgres.DeleteUserRefreshTokens"

	query := `DELETE FROM refresh_tokens WHERE user_id = $1`

	_, err := s.DB.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) DeleteExpiredRefreshTokens(ctx context.Context) error {
	const op = "storage.postgres.DeleteExpiredRefreshTokens"

	query := `DELETE FROM refresh_tokens WHERE expires_at < (NOW() AT TIME ZONE 'UTC')`

	_, err := s.DB.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) DeleteExpiredEmailVerificationTokens(ctx context.Context) error {
	const op = "storage.postgres.DeleteExpiredEmailVerificationTokens"

	query := `DELETE FROM email_verification_tokens WHERE expires_at < NOW()`
	_, err := s.DB.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil

}

func (s *Storage) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	const op = "storage.postgres.GetUserByID"

	var user domain.User
	query := `
	SELECT id, email, password_hash, is_verified, is_premium FROM users
	WHERE id = $1
	`

	err := s.DB.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PassHash,
		&user.IsVerified,
		&user.IsPremium,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s:%w", op, storage.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &user, nil
}

func (s *Storage) CountCategories(ctx context.Context, userID int64) (int, error) {
	const op = "storage.postgres.CountCategories"

	query := `SELECT COUNT(*) FROM categories WHERE user_id = $1`
	var count int
	err := s.DB.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return count, nil
}

func (s *Storage) CountActiveReminders(ctx context.Context, userID int64) (int, error) {
	const op = "storage.postgres.CountActiveReminders"

	query := `SELECT COUNT(*) FROM task_reminders r JOIN tasks t ON t.id=r.task_id WHERE t.user_id=$1 AND r.status='scheduled' AND t.status='pending'`
	var count int
	err := s.DB.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return count, nil
}

func (s *Storage) CreateCategory(ctx context.Context, category domain.Category) (int64, error) {
	const op = "storage.postgres.CreateCategory"
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: begin transaction: %w", op, err)
	}
	defer tx.Rollback() //nolint:errcheck

	var premium bool
	if err := tx.QueryRowContext(ctx, `SELECT is_premium FROM users WHERE id=$1 FOR UPDATE`, category.UserID).Scan(&premium); err != nil {
		return 0, fmt.Errorf("%s: lock user: %w", op, err)
	}
	if !premium {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE user_id=$1`, category.UserID).Scan(&count); err != nil {
			return 0, fmt.Errorf("%s: count categories: %w", op, err)
		}
		if count >= 3 {
			return 0, fmt.Errorf("%s: %w", op, storage.ErrCategoryLimitReached)
		}
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO categories (user_id,name) VALUES ($1,$2) RETURNING id`, category.UserID, category.Name).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return 0, fmt.Errorf("%s: %w", op, storage.ErrCategoryExists)
			}
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s: commit: %w", op, err)
	}
	return id, nil
}

func (s *Storage) GetCategories(ctx context.Context, userID int64) ([]domain.Category, error) {
	const op = "storage.postgres.GetCategories"

	query := `
		SELECT id, user_id, name  FROM categories
		WHERE user_id = $1
	`

	rows, err := s.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close() //nolint:errcheck

	categories := make([]domain.Category, 0)

	for rows.Next() {
		var c domain.Category
		err = rows.Scan(&c.ID, &c.UserID, &c.Name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		categories = append(categories, c)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return categories, nil
}

func (s *Storage) GetCategory(ctx context.Context, userID int64, categoryID int64) (domain.Category, error) {
	const op = "storage.postgres.GetCategory"

	query := `
		SELECT id, user_id, name FROM categories
		WHERE user_id = $1 AND id = $2
	`
	var c domain.Category
	err := s.DB.QueryRowContext(ctx, query, userID, categoryID).Scan(&c.ID, &c.UserID, &c.Name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Category{}, fmt.Errorf("%s: %w", op, storage.ErrCategoryNotFound)
		}
		return domain.Category{}, fmt.Errorf("%s: %w", op, err)
	}
	return c, nil
}

func (s *Storage) DeleteCategory(ctx context.Context, userID int64, categoryID int64) error {
	const op = "storage.postgres.DeleteCategory"

	query := `
		DELETE FROM categories
		WHERE user_id = $1 AND id=$2
		RETURNING id
	`
	var deletedID int64
	err := s.DB.QueryRowContext(ctx, query, userID, categoryID).Scan(&deletedID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s: %w", op, storage.ErrCategoryNotFound)
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *Storage) UpdateCategory(ctx context.Context, userID int64, categoryID int64, c domain.CategoryUpdate) error {
	const op = "storage.postgres.UpdateCategory"
	setValues := make([]string, 0)
	args := make([]any, 0)
	argID := 1
	if c.Name != nil {
		setValues = append(setValues, fmt.Sprintf("name = $%d", argID))
		args = append(args, *c.Name)
		argID++
	}
	if len(setValues) == 0 {
		return nil
	}

	query := fmt.Sprintf(`UPDATE categories SET %s WHERE user_id = $%d AND id = $%d`, strings.Join(setValues, ", "), argID, argID+1) //nolint:gosec

	args = append(args, userID, categoryID)

	res, err := s.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return storage.ErrCategoryNotFound
	}

	return nil
}

func (s *Storage) GetUserStats(ctx context.Context, userID int64) (domain.UserStats, error) {
	const op = "storage.postgres.GetUserStats"

	query :=
		`
	SELECT id, user_id, points, level, total_pomodoros, total_burnt_tasks,
		CASE WHEN last_activity_date >= (CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$1))::date - 1
			THEN COALESCE(current_streak,0) ELSE 0 END,
		best_streak, updated_at FROM user_stats WHERE user_id = $1
	`

	var uS domain.UserStats

	err := s.executor(ctx).QueryRowContext(ctx, query, userID).
		Scan(&uS.ID, &uS.UserID, &uS.Points, &uS.Level, &uS.TotalPomodoros, &uS.TotalBurntTasks, &uS.CurrentStreak, &uS.BestStreak, &uS.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, errInsert := s.executor(ctx).ExecContext(ctx, `INSERT INTO user_stats (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID)
			if errInsert != nil {
				return domain.UserStats{}, fmt.Errorf("%s (init): %w", op, errInsert)
			}
			return s.GetUserStats(ctx, userID)
		}
		return domain.UserStats{}, fmt.Errorf("%s: %w", op, err)
	}

	return uS, nil
}

func (s *Storage) UpdateUserStats(ctx context.Context, userID int64, stats domain.UserStatsUpdate) error {
	const op = "storage.postgres.UpdateStats"

	query := `
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
    `

	_, err := s.DB.ExecContext(ctx, query,
		userID,
		stats.Points,
		stats.Level,
		stats.TotalPomodoros,
		stats.TotalBurntTasks,
		stats.CurrentStreak,
		stats.BestStreak,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *Storage) SaveEmailVerificationToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	const op = "storage.postgres.SaveEmailVerificationToken"
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin transaction: %w", op, err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `UPDATE verification_email_outbox SET status='failed',last_error='Replaced by a newer verification link',lease_token=NULL,lease_until=NULL WHERE user_id=$1 AND status='scheduled'`, userID); err != nil {
		return fmt.Errorf("%s: retire old delivery: %w", op, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO email_verification_tokens (user_id, token, expires_at) VALUES ($1,$2,$3)`, userID, token, expiresAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("%s: %w", op, storage.ErrTokenExists)
		}
		return fmt.Errorf("%s: save token: %w", op, err)
	}
	var email string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM users WHERE id=$1`, userID).Scan(&email); err != nil {
		return fmt.Errorf("%s: load address: %w", op, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO verification_email_outbox(user_id,email,token,expires_at) VALUES($1,$2,$3,$4)`, userID, email, token, expiresAt); err != nil {
		return fmt.Errorf("%s: enqueue email: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

func (s *Storage) GetEmailVerificationToken(ctx context.Context, token string) (domain.EmailVerificationToken, error) {
	const op = "storage.postgres.GetEmailVerificationToken"

	query := `
		SELECT id, user_id, token, expires_at, created_at 
		FROM email_verification_tokens 
		WHERE token = $1
	`

	var t domain.EmailVerificationToken
	err := s.DB.QueryRowContext(ctx, query, token).Scan(&t.ID, &t.UserID, &t.Token, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EmailVerificationToken{}, fmt.Errorf("%s: %w", op, storage.ErrTokenNotFound)
		}
		return domain.EmailVerificationToken{}, fmt.Errorf("%s: %w", op, err)
	}

	return t, nil
}

func (s *Storage) VerifyUserEmail(ctx context.Context, userID int64) error {
	const op = "storage.postgres.VerifyUserEmail"

	query := `
		UPDATE users SET is_verified = true WHERE id = $1
	`

	res, err := s.DB.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if rows == 0 {
		return fmt.Errorf("%s: %w", op, storage.ErrUserNotFound)
	}

	return nil
}

func (s *Storage) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	const op = "storage.postgres.DeleteEmailVerificationToken"

	query := `
		DELETE FROM email_verification_tokens WHERE token = $1
	`
	_, err := s.DB.ExecContext(ctx, query, token)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) UpdateUserScore(ctx context.Context, userID int64, pointsDelta int) error {
	const op = "storage.postgres.UpdateUserScore"
	return s.ApplyStatsDelta(ctx, userID, domain.StatsDelta{PointsDelta: pointsDelta})
}

func (s *Storage) ApplyStatsDelta(ctx context.Context, userID int64, delta domain.StatsDelta) error {
	return s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		return applyStatsDeltaTx(ctx, tx, userID, delta)
	})
}

func (s *Storage) GetAchievements(ctx context.Context, userID int64) ([]domain.Achievement, error) {
	const op = "storage.postgres.GetAchievements"

	if _, err := s.CheckAndUnlockAchievements(ctx, userID); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	query := `SELECT code, unlocked_at FROM achievements WHERE user_id = $1`
	rows, err := s.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	unlockedMap := make(map[string]time.Time)
	for rows.Next() {
		var code string
		var unlockedAt time.Time
		if err := rows.Scan(&code, &unlockedAt); err == nil {
			unlockedMap[code] = unlockedAt
		}
	}

	results := make([]domain.Achievement, len(domain.AvailableAchievements))
	for i, a := range domain.AvailableAchievements {
		item := a
		item.UserID = userID
		if t, ok := unlockedMap[a.Code]; ok {
			item.Unlocked = true
			item.UnlockedAt = &t
		} else {
			item.Unlocked = false
		}
		results[i] = item
	}

	return results, nil
}

func (s *Storage) UnlockAchievement(ctx context.Context, userID int64, code string) (bool, error) {
	var unlocked bool
	err := s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		var err error
		unlocked, err = unlockAchievementTx(ctx, tx, userID, code)
		return err
	})
	if err != nil {
		return false, err
	}
	return unlocked, nil
}

func (s *Storage) CheckAndUnlockAchievements(ctx context.Context, userID int64) ([]domain.Achievement, error) {
	var unlocked []domain.Achievement
	err := s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		var err error
		unlocked, err = checkAndUnlockAchievementsTx(ctx, tx, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return unlocked, nil
}

func (s *Storage) GetDailyQuests(ctx context.Context, userID int64) ([]domain.DailyQuest, error) {
	zone, err := s.GetTimezone(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	var now time.Time
	if err := s.executor(ctx).QueryRowContext(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&now); err != nil {
		return nil, err
	}
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	var createdToday int
	err = s.executor(ctx).QueryRowContext(ctx, `SELECT COUNT(DISTINCT task_id) FROM daily_task_activity WHERE user_id = $1 AND action = 'created' AND recorded_at >= $2`, userID, start).Scan(&createdToday)
	if err != nil {
		return nil, err
	}

	var pomodoroToday int
	err = s.executor(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM pomodoro_sessions WHERE user_id = $1 AND status = 'completed' AND completed_at >= $2 AND completed_at - started_at >= INTERVAL '24 minutes'`, userID, start).Scan(&pomodoroToday)
	if err != nil {
		return nil, err
	}

	var doneToday int
	err = s.executor(ctx).QueryRowContext(ctx, `SELECT COUNT(DISTINCT task_id) FROM daily_task_activity WHERE user_id = $1 AND action = 'completed' AND recorded_at >= $2`, userID, start).Scan(&doneToday)
	if err != nil {
		return nil, err
	}

	quests := []domain.DailyQuest{
		{
			Code:      "create_task",
			Title:     "Планировщик",
			Icon:      "📋",
			Target:    1,
			Current:   createdToday,
			Completed: createdToday >= 1,
			RewardXP:  10,
		},
		{
			Code:      "pomodoro_focus",
			Title:     "Глубокий фокус",
			Icon:      "🍅",
			Target:    1,
			Current:   pomodoroToday,
			Completed: pomodoroToday >= 1,
			RewardXP:  20,
		},
		{
			Code:      "complete_tasks",
			Title:     "Продуктивный рывок",
			Icon:      "✅",
			Target:    3,
			Current:   doneToday,
			Completed: doneToday >= 3,
			RewardXP:  30,
		},
	}

	claims, err := s.dailyClaims(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range quests {
		quests[i].Claimed = claims[quests[i].Code]
	}
	return quests, nil
}

func (s *Storage) GetGamificationProfile(ctx context.Context, userID int64) (domain.GamificationProfile, error) {
	achievements, _ := s.GetAchievements(ctx, userID)

	stats, err := s.GetUserStats(ctx, userID)
	if err != nil {
		return domain.GamificationProfile{}, err
	}

	points := int64(0)
	if stats.Points != nil {
		points = *stats.Points
	}
	streak := int64(0)
	if stats.CurrentStreak != nil {
		streak = *stats.CurrentStreak
	}
	bestStreak := int64(0)
	if stats.BestStreak != nil {
		bestStreak = *stats.BestStreak
	}

	level, currentXP, nextXP, percent, rankTitle, rankIcon := domain.CalculateLevel(points)
	multiplier := domain.GetStreakMultiplier(streak)

	quests, err := s.GetDailyQuests(ctx, userID)
	if err != nil {
		return domain.GamificationProfile{}, err
	}
	claims, err := s.dailyClaims(ctx, userID)
	if err != nil {
		return domain.GamificationProfile{}, err
	}

	unlockedCount := 0
	for _, a := range achievements {
		if a.Unlocked {
			unlockedCount++
		}
	}

	return domain.GamificationProfile{
		Level:             level,
		Points:            points,
		CurrentLevelXP:    currentXP,
		NextLevelXP:       nextXP,
		ProgressPercent:   percent,
		RankTitle:         rankTitle,
		RankIcon:          rankIcon,
		Streak:            streak,
		BestStreak:        bestStreak,
		StreakMultiplier:  multiplier,
		DailyQuests:       quests,
		UnlockedCount:     unlockedCount,
		TotalAchievements: len(domain.AvailableAchievements),
		DailyChestClaimed: claims["daily_chest"],
	}, nil
}
