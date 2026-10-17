//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
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

func TestBusinessActions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	require.NoError(t, err, "Docker is required with -tags integration")
	require.NoError(t, provider.Health(ctx))
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("actions_tests"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	s, err := New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(s.DB, "migrations"))

	userNumber := 0
	newUser := func(t *testing.T, suppressBonuses bool) int64 {
		t.Helper()
		userNumber++
		id, err := s.SaveUser(ctx, fmt.Sprintf("actions_%d@test.local", userNumber), []byte("test"))
		require.NoError(t, err)
		_, err = s.DB.ExecContext(ctx, `INSERT INTO user_stats (user_id) VALUES ($1)`, id)
		require.NoError(t, err)
		if suppressBonuses {
			for _, a := range domain.AvailableAchievements {
				_, err = s.DB.ExecContext(ctx, `INSERT INTO achievements (user_id, code) VALUES ($1, $2)`, id, a.Code)
				require.NoError(t, err)
			}
		}
		return id
	}
	newTask := func(t *testing.T, userID int64) int64 {
		t.Helper()
		id, err := s.SaveTask(ctx, domain.Task{UserID: userID, Title: t.Name(), Status: domain.TaskStatusPending})
		require.NoError(t, err)
		return id
	}
	newSession := func(t *testing.T, userID int64, taskID *int64, eligible bool) int64 {
		t.Helper()
		age := "2 minutes"
		if eligible {
			age = "30 minutes"
		}
		var id int64
		err := s.DB.QueryRowContext(ctx, `INSERT INTO pomodoro_sessions (user_id, task_id, status, started_at, duration_minutes) VALUES ($1, $2, 'active', NOW() - $3::interval, 25) RETURNING id`, userID, taskID, age).Scan(&id)
		require.NoError(t, err)
		return id
	}
	assertStats := func(t *testing.T, userID, wantPoints, wantPomodoros, wantBurnt, wantStreak int64) {
		t.Helper()
		var points, level, pomodoros, burnt, streak int64
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT points, level, total_pomodoros, total_burnt_tasks, current_streak FROM user_stats WHERE user_id = $1`, userID).Scan(&points, &level, &pomodoros, &burnt, &streak))
		require.Equal(t, wantPoints, points)
		require.Equal(t, wantPomodoros, pomodoros)
		require.Equal(t, wantBurnt, burnt)
		require.Equal(t, wantStreak, streak)
		wantLevel, _, _, _, _, _ := domain.CalculateLevel(points)
		require.Equal(t, wantLevel, level)
	}
	done, pending := domain.TaskStatusDone, domain.TaskStatusPending

	t.Run("concurrent single and bulk completion award a task once", func(t *testing.T) {
		userID := newUser(t, true)
		taskID := newTask(t, userID)
		var wg sync.WaitGroup
		errorsCh := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(bulk bool) {
				defer wg.Done()
				if bulk {
					_, err := s.BulkCompleteTasks(ctx, userID, []int64{taskID, taskID})
					errorsCh <- err
				} else {
					errorsCh <- s.UpdateTask(ctx, userID, taskID, domain.TaskUpdate{Status: &done})
				}
			}(i%2 == 0)
		}
		wg.Wait()
		close(errorsCh)
		for err := range errorsCh {
			require.NoError(t, err)
		}
		assertStats(t, userID, 10, 0, 0, 1)
		require.NoError(t, s.UpdateTask(ctx, userID, taskID, domain.TaskUpdate{Status: &pending}))
		require.NoError(t, s.UpdateTask(ctx, userID, taskID, domain.TaskUpdate{Status: &done}))
		assertStats(t, userID, 10, 0, 0, 1)
		task, err := s.GetTask(ctx, userID, taskID)
		require.NoError(t, err)
		require.True(t, task.RewardClaimed)
	})

	t.Run("bulk rewards each eligible owned task and records completion time", func(t *testing.T) {
		userID := newUser(t, true)
		otherUser := newUser(t, true)
		first, second := newTask(t, userID), newTask(t, userID)
		foreign := newTask(t, otherUser)
		_, err := s.DB.ExecContext(ctx, `UPDATE tasks SET updated_at = CURRENT_DATE - INTERVAL '2 days' WHERE user_id = $1`, userID)
		require.NoError(t, err)
		affected, err := s.BulkCompleteTasks(ctx, userID, []int64{first, first, second, foreign})
		require.NoError(t, err)
		require.EqualValues(t, 2, affected)
		assertStats(t, userID, 20, 0, 0, 1)
		var count int
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE user_id = $1 AND reward_claimed AND updated_at >= CURRENT_DATE`, userID).Scan(&count))
		require.Equal(t, 2, count)
		affected, err = s.BulkCompleteTasks(ctx, userID, []int64{first, second})
		require.NoError(t, err)
		require.Zero(t, affected)
		assertStats(t, userID, 20, 0, 0, 1)
	})

	t.Run("pomodoro and task rewards are additive and cannot be replayed", func(t *testing.T) {
		userID := newUser(t, true)
		taskID := newTask(t, userID)
		sessionID := newSession(t, userID, &taskID, true)
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- s.FinishPomodoroSession(ctx, userID, sessionID, "completed", true) }()
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, storage.ErrSessionNotFound)
			}
		}
		require.Equal(t, 1, successes)
		assertStats(t, userID, 30, 1, 0, 1)
		task, err := s.GetTask(ctx, userID, taskID)
		require.NoError(t, err)
		require.EqualValues(t, 1, task.PomodorosTaken)
		require.True(t, task.RewardClaimed)
		next := newSession(t, userID, &taskID, true)
		require.NoError(t, s.FinishPomodoroSession(ctx, userID, next, "completed", true))
		assertStats(t, userID, 50, 2, 0, 1)
		abandoned := newSession(t, userID, &taskID, false)
		require.NoError(t, s.FinishPomodoroSession(ctx, userID, abandoned, "abandoned", true))
		assertStats(t, userID, 20, 2, 1, 0)
		require.ErrorIs(t, s.FinishPomodoroSession(ctx, userID, abandoned, "abandoned", true), storage.ErrSessionNotFound)
		assertStats(t, userID, 20, 2, 1, 0)
	})

	t.Run("short sessions and abandoned free sessions do not earn rewards", func(t *testing.T) {
		userID := newUser(t, true)
		taskID := newTask(t, userID)
		sessionID := newSession(t, userID, &taskID, false)
		require.NoError(t, s.FinishPomodoroSession(ctx, userID, sessionID, "completed", false))
		assertStats(t, userID, 0, 0, 0, 0)
		task, err := s.GetTask(ctx, userID, taskID)
		require.NoError(t, err)
		require.Zero(t, task.PomodorosTaken)
		free := newSession(t, userID, nil, false)
		require.NoError(t, s.FinishPomodoroSession(ctx, userID, free, "abandoned", false))
		assertStats(t, userID, 0, 0, 0, 0)
		quests, err := s.GetDailyQuests(ctx, userID)
		require.NoError(t, err)
		require.Zero(t, quests[1].Current)
	})

	t.Run("starting and finishing checks ownership and exact session ID", func(t *testing.T) {
		userID, foreignUser := newUser(t, true), newUser(t, true)
		taskID := newTask(t, foreignUser)
		_, err := s.StartPomodoroSession(ctx, userID, &taskID)
		require.ErrorIs(t, err, storage.ErrTaskNotFound)
		sessionID := newSession(t, foreignUser, &taskID, true)
		require.ErrorIs(t, s.FinishPomodoroSession(ctx, userID, sessionID, "completed", true), storage.ErrSessionNotFound)
		own := newSession(t, userID, nil, true)
		require.ErrorIs(t, s.FinishPomodoroSession(ctx, userID, sessionID, "completed", true), storage.ErrSessionNotFound)
		active, err := s.GetActivePomodoroSession(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, own, active.ID)
		assertStats(t, userID, 0, 0, 0, 0)
	})

	t.Run("parallel starts create only one active session", func(t *testing.T) {
		userID := newUser(t, true)
		results := make(chan error, 8)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := s.StartPomodoroSession(ctx, userID, nil); results <- err }()
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, storage.ErrSessionActive)
			}
		}
		require.Equal(t, 1, successes)
	})

	t.Run("stats failure rolls back task claim session and bulk changes", func(t *testing.T) {
		userID := newUser(t, true)
		taskID := newTask(t, userID)
		sessionID := newSession(t, userID, &taskID, true)
		_, err := s.DB.ExecContext(ctx, `ALTER TABLE user_stats ADD CONSTRAINT test_stats_failure CHECK (points <= 0) NOT VALID`)
		require.NoError(t, err)
		defer func() {
			_, err := s.DB.ExecContext(ctx, `ALTER TABLE user_stats DROP CONSTRAINT test_stats_failure`)
			require.NoError(t, err)
		}()
		require.Error(t, s.UpdateTask(ctx, userID, taskID, domain.TaskUpdate{Status: &done}))
		_, err = s.BulkCompleteTasks(ctx, userID, []int64{taskID})
		require.Error(t, err)
		require.Error(t, s.FinishPomodoroSession(ctx, userID, sessionID, "completed", true))
		task, err := s.GetTask(ctx, userID, taskID)
		require.NoError(t, err)
		require.Equal(t, pending, task.Status)
		require.False(t, task.RewardClaimed)
		require.Zero(t, task.PomodorosTaken)
		active, err := s.GetActivePomodoroSession(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, sessionID, active.ID)
		assertStats(t, userID, 0, 0, 0, 0)
	})

	t.Run("achievement bonus is atomic and does not extend a streak on reading", func(t *testing.T) {
		userID := newUser(t, false)
		_, err := s.DB.ExecContext(ctx, `UPDATE user_stats SET current_streak = 3, best_streak = 3, last_activity_date = CURRENT_DATE - 1 WHERE user_id = $1`, userID)
		require.NoError(t, err)
		var before sql.NullTime
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT last_activity_date FROM user_stats WHERE user_id = $1`, userID).Scan(&before))
		require.NoError(t, func() error { _, err := s.GetAchievements(ctx, userID); return err }())
		var after sql.NullTime
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT last_activity_date FROM user_stats WHERE user_id = $1`, userID).Scan(&after))
		require.Equal(t, before, after)
		var points int64
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT points FROM user_stats WHERE user_id = $1`, userID).Scan(&points))
		require.Positive(t, points)
		_, err = s.GetAchievements(ctx, userID)
		require.NoError(t, err)
		assertStats(t, userID, points, 0, 0, 3)
	})

	t.Run("task deletion races with finishing without deadlock or partial rewards", func(t *testing.T) {
		for i := 0; i < 8; i++ {
			userID := newUser(t, true)
			taskID := newTask(t, userID)
			sessionID := newSession(t, userID, &taskID, true)
			finished, deleted := make(chan error, 1), make(chan error, 1)
			go func() { finished <- s.FinishPomodoroSession(ctx, userID, sessionID, "completed", true) }()
			go func() { deleted <- s.DeleteTask(ctx, userID, taskID) }()
			require.NoError(t, <-deleted)
			finishErr := <-finished
			var status string
			var linkedTask sql.NullInt64
			require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT status, task_id FROM pomodoro_sessions WHERE id = $1`, sessionID).Scan(&status, &linkedTask))
			require.False(t, linkedTask.Valid)
			if finishErr == nil {
				require.Equal(t, "completed", status)
				assertStats(t, userID, 30, 1, 0, 1)
			} else {
				require.True(t, errors.Is(finishErr, storage.ErrSessionNotFound) || errors.Is(finishErr, storage.ErrTaskNotFound), finishErr)
				require.Equal(t, "abandoned", status)
				assertStats(t, userID, 0, 0, 0, 0)
			}
		}
	})
}
