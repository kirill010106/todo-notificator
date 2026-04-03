package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/kirill010106/todo-notificator/notifiers/shared/storage"

	_ "github.com/lib/pq"
)

type Storage struct {
	DB *sql.DB
}

func New(DBUrl string) (*Storage, error) {
	const op = "storage.postgres.New"

	db, err := sql.Open("postgres", DBUrl)
	if err != nil {
		return nil, fmt.Errorf("%s:%w", op, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("%s ping: %w", op, err)
	}

	return &Storage{
		DB: db,
	}, nil
}

func (s *Storage) Close() error {
	return s.DB.Close()
}

func (s *Storage) GetPendingTasksWithDeadline(ctx context.Context) ([]domain.Task, error) {
	const op = "storage.postgres.GetPendingTasksWithDeadlines"

	query :=
		`
	SELECT id, user_id, title, description, reminder_at, deadline, status
	FROM tasks
	WHERE status = 'pending'
		AND deadline IS NOT NULL
		AND deadline > NOW()
	ORDER BY deadline ASC
	`

	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.Description, &t.ReminderAt, &t.Deadline, &t.Status,
		); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (s *Storage) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	const op = "storage.postgres.GetUserByID"

	var user domain.User
	query := `
	SELECT id, email FROM users WHERE id = $1
	`

	err := s.DB.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s:%w", op, storage.ErrUserNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &user, nil

}

func (s *Storage) GetPendingTasksWithUsers(ctx context.Context) ([]domain.TaskWithUser, error) {
	const op = "storage.postgres.GetPendingTasksWithUsers"
	query := `
	SELECT t.id, t.user_id, t.title, t.description, t.reminder_at, t.deadline, t.status, u.id, u.email
	FROM tasks t
	JOIN users u ON u.id = t.user_id
	WHERE t.status = 'pending'
	AND t.is_notified = false
	AND t.reminder_at IS NOT NULL
	AND t.reminder_at <= NOW()
	AND u.is_verified = true
	ORDER BY t.reminder_at ASC
	`

	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var res []domain.TaskWithUser
	for rows.Next() {
		var tw domain.TaskWithUser
		if err := rows.Scan(
			&tw.Task.ID,
			&tw.Task.UserID,
			&tw.Task.Title,
			&tw.Task.Description,
			&tw.Task.ReminderAt,
			&tw.Task.Deadline,
			&tw.Task.Status,
			&tw.User.ID,
			&tw.User.Email,
		); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		res = append(res, tw)
	}
	return res, rows.Err()
}

func (s *Storage) GetTasksDueBetween(ctx context.Context, from, to time.Time) ([]domain.Task, error) {
	const op = "storage.postgres.GetTasksDueBetween"

	query := `
		SELECT id, user_id, title, description, reminder_at, deadline, status
		FROM tasks
		WHERE status = 'pending'
			AND deadline IS NOT NULL
			AND deadline BETWEEN $1 AND $2
			ORDER BY deadline ASC
	`

	rows, err := s.DB.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.Description, &t.ReminderAt, &t.Deadline, &t.Status,
		); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (s *Storage) MarkTaskAsNotified(ctx context.Context, taskID int64) error {
	const op = "storage.postgres.MarkTaskAsNotified"

	query := `
	UPDATE tasks SET is_notified = true WHERE id = $1
	`

	_, err := s.DB.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
