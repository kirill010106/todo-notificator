package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestStartPomodoroSession_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(10)

	// 1. Check for absence of active session
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id from pomodoro_sessions WHERE user_id = $1 AND status = $2`)).
		WithArgs(userID, domain.PomodoroStatusActive).
		WillReturnError(sql.ErrNoRows) // Simulate that session is not found

	// 2. Creation
	now := time.Now()
	query := regexp.QuoteMeta(`
        INSERT INTO pomodoro_sessions (user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at)
        VALUES ($1, $2, $3, NOW(), 25, 0, NOW())
        RETURNING id, user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at
    `)

	mock.ExpectQuery(query).
		WithArgs(userID, &taskID, domain.PomodoroStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "task_id", "status", "started_at", "duration_minutes", "breaks_used", "created_at"}).
			AddRow(1, userID, taskID, domain.PomodoroStatusActive, now, 25, 0, now))

	session, err := s.StartPomodoroSession(context.Background(), userID, &taskID)

	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, int64(1), session.ID)
	require.Equal(t, userID, session.UserID)
	require.Equal(t, domain.PomodoroStatusActive, session.Status)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestStartPomodoroSession_AlreadyActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id from pomodoro_sessions WHERE user_id = $1 AND status = $2`)).
		WithArgs(userID, domain.PomodoroStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99)) // Active session found

	session, err := s.StartPomodoroSession(context.Background(), userID, nil)

	require.ErrorIs(t, err, storage.ErrSessionActive)
	require.Nil(t, session)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestAddPomodoroBreak_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	sessionID := int64(1)

	query := regexp.QuoteMeta(`
        UPDATE pomodoro_sessions 
        SET breaks_used = 1 
        WHERE id = $1 AND breaks_used = 0 AND status = $2 AND user_id = $3
    `)

	mock.ExpectExec(query).
		WithArgs(sessionID, domain.PomodoroStatusActive, userID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 affected row

	err = s.AddPomodoroBreak(context.Background(), userID, sessionID)
	require.NoError(t, err)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestAddPomodoroBreak_Exhausted(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	sessionID := int64(1)

	query := regexp.QuoteMeta(`
        UPDATE pomodoro_sessions 
        SET breaks_used = 1 
        WHERE id = $1 AND breaks_used = 0 AND status = $2 AND user_id = $3
    `)

	mock.ExpectExec(query).
		WithArgs(sessionID, domain.PomodoroStatusActive, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 affected rows

	err = s.AddPomodoroBreak(context.Background(), userID, sessionID)
	require.ErrorIs(t, err, storage.ErrBreakExhausted)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestStopPomodoroSession_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	sessionID := int64(1)

	query := regexp.QuoteMeta(`
        UPDATE pomodoro_sessions 
        SET status = $1, completed_at = NOW() 
        WHERE id = $2 AND status = $3 AND user_id = $4
    `)

	mock.ExpectExec(query).
		WithArgs(domain.PomodoroStatusCompleted, sessionID, domain.PomodoroStatusActive, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.StopPomodoroSession(context.Background(), userID, sessionID, domain.PomodoroStatusCompleted)
	require.NoError(t, err)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestStopPomodoroSession_NotActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	sessionID := int64(1)

	query := regexp.QuoteMeta(`
        UPDATE pomodoro_sessions 
        SET status = $1, completed_at = NOW() 
        WHERE id = $2 AND status = $3 AND user_id = $4
    `)

	mock.ExpectExec(query).
		WithArgs(domain.PomodoroStatusCompleted, sessionID, domain.PomodoroStatusActive, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 affected rows

	err = s.StopPomodoroSession(context.Background(), userID, sessionID, domain.PomodoroStatusCompleted)
	require.ErrorIs(t, err, storage.ErrSessionNotFound)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestStopPomodoroSession_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	sessionID := int64(1)

	query := regexp.QuoteMeta(`
        UPDATE pomodoro_sessions 
        SET status = $1, completed_at = NOW() 
        WHERE id = $2 AND status = $3 AND user_id = $4
    `)

	mock.ExpectExec(query).
		WithArgs(domain.PomodoroStatusCompleted, sessionID, domain.PomodoroStatusActive, userID).
		WillReturnError(sql.ErrConnDone)

	err = s.StopPomodoroSession(context.Background(), userID, sessionID, domain.PomodoroStatusCompleted)
	require.Error(t, err)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestGetActivePomodoroSession_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	taskID := int64(10)
	now := time.Now()

	query := regexp.QuoteMeta(`
		SELECT id, user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at
		FROM pomodoro_sessions
		WHERE user_id = $1 AND status = $2
	`)

	mock.ExpectQuery(query).
		WithArgs(userID, domain.PomodoroStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "task_id", "status", "started_at", "duration_minutes", "breaks_used", "created_at"}).
			AddRow(int64(1), userID, taskID, domain.PomodoroStatusActive, now, 25, 0, now))

	session, err := s.GetActivePomodoroSession(context.Background(), userID)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, int64(1), session.ID)
	require.Equal(t, userID, session.UserID)
	require.Equal(t, domain.PomodoroStatusActive, session.Status)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestGetActivePomodoroSession_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)

	query := regexp.QuoteMeta(`
		SELECT id, user_id, task_id, status, started_at, duration_minutes, breaks_used, created_at
		FROM pomodoro_sessions
		WHERE user_id = $1 AND status = $2
	`)

	mock.ExpectQuery(query).
		WithArgs(userID, domain.PomodoroStatusActive).
		WillReturnError(sql.ErrNoRows)

	session, err := s.GetActivePomodoroSession(context.Background(), userID)
	require.Error(t, err)
	require.Nil(t, session)
	require.ErrorIs(t, err, storage.ErrSessionNotFound)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestAddPomodoroBreak_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	s := &Storage{DB: db}
	userID := int64(42)
	sessionID := int64(1)

	query := regexp.QuoteMeta(`
        UPDATE pomodoro_sessions 
        SET breaks_used = 1 
        WHERE id = $1 AND breaks_used = 0 AND status = $2 AND user_id = $3
    `)

	mock.ExpectExec(query).
		WithArgs(sessionID, domain.PomodoroStatusActive, userID).
		WillReturnError(sql.ErrConnDone)

	err = s.AddPomodoroBreak(context.Background(), userID, sessionID)
	require.Error(t, err)

	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}
