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
	"github.com/kirill010106/todo-notificator/backend/internal/domain"
	"github.com/kirill010106/todo-notificator/backend/internal/storage"
	_ "github.com/lib/pq"
)

type Storage struct {
	Db *sql.DB
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
	return &Storage{Db: db}, nil
}

func (s *Storage) Close() error {
	if s.Db != nil {
		return s.Db.Close()
	}
	return nil
}

func (s *Storage) SaveTask(ctx context.Context, task domain.Task) (int64, error) {
	const op = "storage.postgres.saveTask"

	query := `
INSERT INTO tasks (user_id, title, description, deadline, status, is_notified)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id`
	var id int64

	err := s.Db.QueryRowContext(ctx, query, task.UserID, task.Title, task.Description, task.Deadline, task.Status, task.IsNotified).Scan(&id)

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

func (s *Storage) GetTasks(ctx context.Context, userID int64) ([]domain.Task, error) {
	const op = "storage.postgres.getTasks"

	query := `
SELECT id, user_id, title, description, deadline, status, is_notified
FROM tasks
WHERE user_id = $1`

	rows, err := s.Db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0)

	for rows.Next() {
		var t domain.Task
		err := rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Deadline, &t.Status, &t.IsNotified)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		tasks = append(tasks, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (s *Storage) DeleteTask(ctx context.Context, userID int64, taskID int64) error {
	const op = "storage.postgres.DeleteTask"

	query := `
		DELETE FROM tasks 
		       WHERE user_id=$1 AND id=$2
		RETURNING id
`
	var deletedID int64
	err := s.Db.QueryRowContext(ctx, query, userID, taskID).Scan(&deletedID)

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
	args := make([]interface{}, 0)
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

	if len(setValues) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
			UPDATE tasks
			SET %s
			WHERE user_id = $%d AND id = $%d
`, strings.Join(setValues, ", "), argID, argID+1)

	args = append(args, userID, taskID)

	res, err := s.Db.ExecContext(ctx, query, args...)
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

	query := `SELECT id, email, password_hash from users WHERE email = $1`

	var user domain.User
	err := s.Db.QueryRowContext(ctx, query, email).Scan(&user.ID, &user.Email, &user.PassHash)
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
	err := s.Db.QueryRowContext(ctx, query, email, passHash).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
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

	_, err := s.Db.ExecContext(ctx, query, userID, token, expiresAt)
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
	err := s.Db.QueryRowContext(ctx, query, token).Scan(
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
	res, err := s.Db.ExecContext(ctx, query, token)
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

	_, err := s.Db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	const op = "storage.postgres.GetUserByID"

	var user domain.User
	query := `
	SELECT id, email, password_hash FROM users
	WHERE id = $1
	`

	err := s.Db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PassHash)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s:%w", op, storage.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &user, err

}
