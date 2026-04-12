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

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
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

func (s *Storage) SaveTask(ctx context.Context, task domain.Task) (int64, error) {
	const op = "storage.postgres.saveTask"

	var categoryArg any = nil
	if task.CategoryID != nil && *task.CategoryID > 0 {
		var exists bool
		err := s.DB.QueryRowContext(
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
INSERT INTO tasks (user_id, title, description, deadline, reminder_at, status, is_notified, category_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id`
	var id int64

	err := s.DB.QueryRowContext(ctx, query, task.UserID, task.Title, task.Description, task.Deadline, task.ReminderAt, task.Status, task.IsNotified, categoryArg).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return 0, fmt.Errorf("%s: %w", op, storage.ErrTaskExists)
			}
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (s *Storage) GetTask(ctx context.Context, userID int64, taskID int64) (domain.Task, error) {
	const op = "storage.postgres.GetTask"

	query := `
		SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken, reward_claimed
		FROM tasks
		WHERE user_id = $1 AND id = $2
	`
	var t domain.Task
	err := s.DB.QueryRowContext(ctx, query, userID, taskID).Scan(
		&t.ID,
		&t.UserID,
		&t.Title,
		&t.Description,
		&t.Deadline,
		&t.ReminderAt,
		&t.Status,
		&t.IsNotified,
		&t.CategoryID,
		&t.PomodorosTaken,
		&t.RewardClaimed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, fmt.Errorf("%s: %w", op, storage.ErrTaskNotFound)
		}
		return domain.Task{}, fmt.Errorf("%s: %w", op, err)
	}
	return t, nil
}

func (s *Storage) GetTasks(ctx context.Context, userID int64, limit, offset int) ([]domain.Task, int, error) {
	const op = "storage.postgres.GetTasks"

	countQuery := `
		SELECT COUNT(*) FROM tasks WHERE user_id = $1
	`

	var total int
	if err := s.DB.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	dataQuery := `
SELECT id, user_id, title, description, deadline, reminder_at, status, is_notified, category_id, pomodoro_taken, reward_claimed
FROM tasks
	WHERE user_id = $1 ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`

	rows, err := s.DB.QueryContext(ctx, dataQuery, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close() //nolint:errcheck

	tasks := make([]domain.Task, 0, limit)

	for rows.Next() {
		var t domain.Task
		err = rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Deadline, &t.ReminderAt, &t.Status, &t.IsNotified, &t.CategoryID, &t.PomodorosTaken, &t.RewardClaimed)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", op, err)
		}
		tasks = append(tasks, t)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, total, nil
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

	if t.ReminderAt != nil {
		setValues = append(setValues, fmt.Sprintf("reminder_at = $%d", argID))
		args = append(args, *t.ReminderAt)
		argID++
	}

	if t.CategoryID != nil {
		var exists bool
		err := s.DB.QueryRowContext(
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

	if len(setValues) == 0 {
		return nil
	}

	query := fmt.Sprintf(`UPDATE tasks SET %s WHERE user_id = $%d AND id = $%d`, strings.Join(setValues, ", "), argID, argID+1) //nolint:gosec

	args = append(args, userID, taskID)

	res, err := s.DB.ExecContext(ctx, query, args...)
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

	query := `SELECT id, email, password_hash, is_verified FROM users WHERE email = $1`

	var user domain.User
	err := s.DB.QueryRowContext(ctx, query, email).Scan(&user.ID, &user.Email, &user.PassHash, &user.IsVerified)
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

func (s *Storage) SaveRefreshToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	const op = "storage.postgres.SaveRefreshToken"

	query := ` 
		INSERT INTO refresh_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`

	_, err := s.DB.ExecContext(ctx, query, userID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *Storage) GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	const op = "storage.postgres.GetRefreshToken"

	query := `
	SELECT id, user_id, token, expires_at, created_at
	 FROM refresh_tokens
	 WHERE token = $1 AND expires_at > NOW()
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

	query := `DELETE FROM refresh_tokens WHERE expires_at < NOW()`

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
	SELECT id, email, password_hash, is_verified FROM users
	WHERE id = $1
	`

	err := s.DB.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PassHash,
		&user.IsVerified,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s:%w", op, storage.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &user, nil

}

func (s *Storage) CreateCategory(ctx context.Context, category domain.Category) (int64, error) {
	const op = "storage.postgres.CreateCategory"

	query := `
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		RETURNING id
	`
	var id int64

	err := s.DB.QueryRowContext(ctx, query, category.UserID, category.Name).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return 0, fmt.Errorf("%s: %w", op, storage.ErrCategoryExists)
			}
		}
		return 0, fmt.Errorf("%s: %w", op, err)
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
	SELECT id, user_id, points, level, total_pomodoros, total_burnt_tasks, current_streak, best_streak, updated_at FROM user_stats WHERE user_id = $1
	`

	var uS domain.UserStats

	err := s.DB.QueryRowContext(ctx, query, userID).
		Scan(&uS.ID, &uS.UserID, &uS.Points, &uS.Level, &uS.TotalPomodoros, &uS.TotalBurntTasks, &uS.CurrentStreak, &uS.BestStreak, &uS.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, errInsert := s.DB.ExecContext(ctx, `INSERT INTO user_stats (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID)
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

	query := `
		INSERT INTO email_verification_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`

	_, err := s.DB.ExecContext(ctx, query, userID, token, expiresAt)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("%s: %w", op, storage.ErrTokenExists) // Or distinct error "token already exists"
		}
		return fmt.Errorf("%s: %w", op, err)
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

	query := `
    UPDATE user_stats 
    SET 
		points = points + $1,
		level = GREATEST(1, ((points + $1) / 100) + 1)
    WHERE user_id = $2
`
	_, err := s.DB.ExecContext(ctx, query, pointsDelta, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *Storage) ApplyStatsDelta(ctx context.Context, userID int64, delta domain.StatsDelta) error {
	const op = "storage.postgres.ApplyStatsDelta"

	query := `
		UPDATE user_stats
		SET
			points = points + $1
			level = GREATEST(1, ((points + $1) / 100) + 1)
			total_pomodoros = total_pomodoros + $2
			total_burnt_tasks = total_burnt_tasks + $3

			current_streak = CASE
			WHEN $4 = true THEN 0
			WHEN $5 = TRUE THEN curent_streak +1
			ELSE current_streak
		END,
		best_streak = GREATEST(best_streak, CASE
			WHEN $5 = TRUE THEN current_streak + 1
			ELSE current_streak
		END)
		WHERE user_id = $6
	`

	_, err := s.DB.ExecContext(ctx, query,
		delta.PointsDelta,
		delta.PomodorosDelta,
		delta.BurntTasksDelta,
		delta.ResetStreak,
		delta.IncrementStreak,
		userID,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
