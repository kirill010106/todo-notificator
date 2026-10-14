//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Run with: go test -tags integration ./internal/storage/postgres -run TestDeletionRelationships -v
// Always uses a disposable database; never connects to the application's DB.
func TestDeletionRelationships(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	require.NoError(t, err, "Docker is required with -tags integration")
	require.NoError(t, provider.Health(ctx))
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("deletion_tests"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	s, err := New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(s.DB, "migrations", 17))

	userNumber := 0
	newUser := func(t *testing.T) int64 {
		t.Helper()
		userNumber++
		id, err := s.SaveUser(ctx, fmt.Sprintf("deletion_%d@test.local", userNumber), []byte("test"))
		require.NoError(t, err)
		_, err = s.DB.ExecContext(ctx, `INSERT INTO user_stats (user_id, points, current_streak, best_streak) VALUES ($1, 100, 3, 4)`, id)
		require.NoError(t, err)
		return id
	}
	newTask := func(t *testing.T, userID int64, categoryID *int64) int64 {
		t.Helper()
		id, err := s.SaveTask(ctx, domain.Task{UserID: userID, Title: t.Name(), Status: domain.TaskStatusPending, CategoryID: categoryID})
		require.NoError(t, err)
		return id
	}
	newSession := func(t *testing.T, userID int64, taskID *int64, status string) int64 {
		t.Helper()
		var id int64
		err := s.DB.QueryRowContext(ctx, `INSERT INTO pomodoro_sessions (user_id, task_id, status, started_at, completed_at, duration_minutes) VALUES ($1, $2, $3::varchar, NOW() - INTERVAL '25 minutes', CASE WHEN $3::varchar = 'active' THEN NULL ELSE TIMESTAMP '2026-01-01 12:00:00' END, 25) RETURNING id`, userID, taskID, status).Scan(&id)
		require.NoError(t, err)
		return id
	}
	statsSnapshot := func(t *testing.T, userID int64) string {
		t.Helper()
		var snapshot string
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT to_jsonb(us)::text FROM user_stats us WHERE user_id = $1`, userID).Scan(&snapshot))
		return snapshot
	}
	assertSession := func(t *testing.T, id int64, wantStatus string, wantTask *int64) {
		t.Helper()
		var status string
		var taskID sql.NullInt64
		var completedAt sql.NullTime
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT status, task_id, completed_at FROM pomodoro_sessions WHERE id = $1`, id).Scan(&status, &taskID, &completedAt))
		require.Equal(t, wantStatus, status)
		if wantTask == nil {
			require.False(t, taskID.Valid)
		} else {
			require.True(t, taskID.Valid)
			require.Equal(t, *wantTask, taskID.Int64)
		}
		require.Equal(t, wantStatus != domain.PomodoroStatusActive, completedAt.Valid)
	}

	// Verify upgrading a DB that already contains tasks and active/history sessions.
	userID := newUser(t)
	taskID := newTask(t, userID, nil)
	activeID := newSession(t, userID, &taskID, domain.PomodoroStatusActive)
	historyID := newSession(t, userID, &taskID, domain.PomodoroStatusCompleted)
	statsBefore := statsSnapshot(t, userID)
	require.NoError(t, goose.Up(s.DB, "migrations"))
	require.NoError(t, s.DeleteTask(ctx, userID, taskID))
	assertSession(t, activeID, domain.PomodoroStatusAbandoned, nil)
	assertSession(t, historyID, domain.PomodoroStatusCompleted, nil)
	var historyCompletedAt time.Time
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT completed_at FROM pomodoro_sessions WHERE id = $1`, historyID).Scan(&historyCompletedAt))
	require.Equal(t, "2026-01-01 12:00:00", historyCompletedAt.Format("2006-01-02 15:04:05"))
	require.Equal(t, statsBefore, statsSnapshot(t, userID))
	_, err = s.GetActivePomodoroSession(ctx, userID)
	require.ErrorIs(t, err, storage.ErrSessionNotFound)

	t.Run("category deletion preserves tasks and their active timer", func(t *testing.T) {
		userID := newUser(t)
		categoryID, err := s.CreateCategory(ctx, domain.Category{UserID: userID, Name: "Study"})
		require.NoError(t, err)
		taskID := newTask(t, userID, &categoryID)
		sessionID := newSession(t, userID, &taskID, domain.PomodoroStatusActive)
		require.NoError(t, s.DeleteCategory(ctx, userID, categoryID))
		task, err := s.GetTask(ctx, userID, taskID)
		require.NoError(t, err)
		require.Nil(t, task.CategoryID)
		assertSession(t, sessionID, domain.PomodoroStatusActive, &taskID)
	})

	t.Run("bulk deletion stops only timers belonging to deleted tasks", func(t *testing.T) {
		userID := newUser(t)
		otherUserID := newUser(t)
		first := newTask(t, userID, nil)
		second := newTask(t, userID, nil)
		other := newTask(t, otherUserID, nil)
		firstSession := newSession(t, userID, &first, domain.PomodoroStatusActive)
		secondSession := newSession(t, userID, &second, domain.PomodoroStatusAbandoned)
		otherSession := newSession(t, otherUserID, &other, domain.PomodoroStatusActive)
		freeSession := newSession(t, userID, nil, domain.PomodoroStatusActive)
		statsBefore := statsSnapshot(t, userID)
		affected, err := s.BulkDeleteTasks(ctx, userID, []int64{first, second, other})
		require.NoError(t, err)
		require.EqualValues(t, 2, affected)
		assertSession(t, firstSession, domain.PomodoroStatusAbandoned, nil)
		assertSession(t, secondSession, domain.PomodoroStatusAbandoned, nil)
		assertSession(t, otherSession, domain.PomodoroStatusActive, &other)
		assertSession(t, freeSession, domain.PomodoroStatusActive, nil)
		require.Equal(t, statsBefore, statsSnapshot(t, userID))
	})

	t.Run("rollback restores task and active session together", func(t *testing.T) {
		userID := newUser(t)
		taskID := newTask(t, userID, nil)
		sessionID := newSession(t, userID, &taskID, domain.PomodoroStatusActive)
		tx, err := s.DB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		require.NoError(t, err)
		require.NoError(t, tx.Rollback())
		_, err = s.GetTask(ctx, userID, taskID)
		require.NoError(t, err)
		assertSession(t, sessionID, domain.PomodoroStatusActive, &taskID)
	})

	// Verify rollback and reapplication of the new migration.
	require.NoError(t, goose.Down(s.DB, "migrations"))
	require.NoError(t, goose.Up(s.DB, "migrations"))
}
