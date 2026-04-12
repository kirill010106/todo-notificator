package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

// StartPomodoroSession creates a new active Pomodoro session.
// If the user already has an active session, storage.ErrSessionActive is returned.
func (s *Storage) StartPomodoroSession(ctx context.Context, userID int64, taskID *int64) (*domain.PomodoroSession, error) {
	const op = "storage.postgres.StartPomodoroSession"

	var activeID int64

	err := s.DB.QueryRowContext(ctx, "SELECT id from pomodoro_sessions WHERE user_id = $1 AND status = $2", userID, domain.PomodoroStatusActive).Scan(&activeID)
	if err == nil {
		return nil, fmt.Errorf("%s: %w", op, storage.ErrSessionActive)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	query := `
        INSERT INTO pomodoro_sessions (user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at)
        VALUES ($1, $2, $3, NOW(), 25, 0, NOW())
        RETURNING id, user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at
    `

	session := &domain.PomodoroSession{}
	err = s.DB.QueryRowContext(ctx, query, userID, taskID, domain.PomodoroStatusActive).Scan(
		&session.ID,
		&session.UserID,
		&session.TaskID,
		&session.Status,
		&session.StartedAt,
		&session.DurationMinutes,
		&session.BreaksUsed,
		&session.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return session, nil
}

func (s *Storage) GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error) {
	const op = "storage.postgres.GetActivePomodoroSession"

	query := `
        SELECT id, user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at
        FROM pomodoro_sessions 
        WHERE user_id = $1 AND status = $2
    `

	session := &domain.PomodoroSession{}
	err := s.DB.QueryRowContext(ctx, query, userID, domain.PomodoroStatusActive).Scan(
		&session.ID,
		&session.UserID,
		&session.TaskID,
		&session.Status,
		&session.StartedAt,
		&session.DurationMinutes,
		&session.BreaksUsed,
		&session.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, storage.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return session, nil
}

func (s *Storage) AddPomodoroBreak(ctx context.Context, sessionID int64) error {
	const op = "storage.postgres.AddPomodoroBreak"

	query := `
        UPDATE pomodoro_sessions 
        SET breaks_used = 1 
        WHERE id = $1 AND breaks_used = 0 AND status = $2
    `

	res, err := s.DB.ExecContext(ctx, query, sessionID, domain.PomodoroStatusActive)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if affected == 0 {
		return fmt.Errorf("%s: %w", op, storage.ErrBreakExhausted)
	}

	return nil
}

func (s *Storage) StopPomodoroSession(ctx context.Context, sessionID int64, finalStatus string) error {
	const op = "storage.postgres.StopPomodoroSession"

	query := `
        UPDATE pomodoro_sessions 
        SET status = $1, completed_at = NOW() 
        WHERE id = $2 AND status = $3
    `

	res, err := s.DB.ExecContext(ctx, query, finalStatus, sessionID, domain.PomodoroStatusActive)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if affected == 0 {
		return fmt.Errorf("%s: %w", op, storage.ErrSessionNotFound)
	}

	return nil
}
