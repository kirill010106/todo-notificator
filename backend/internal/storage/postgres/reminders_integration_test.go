//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	queue "github.com/kirill010106/todo-notificator/notifiers/shared/storage/postgres"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestTaskReminders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("reminders"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	s, err := New(dsn)
	require.NoError(t, err)
	defer s.Close()
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(s.DB, "migrations", 20))
	user, err := s.SaveUser(ctx, "reminders@test.local", []byte("test"))
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE users SET is_verified=true WHERE id=$1`, user)
	require.NoError(t, err)
	var base time.Time
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT date_trunc('minute',CURRENT_TIMESTAMP)+INTERVAL '3 days'`).Scan(&base))
	base = base.UTC() // the old browser API serialized timestamps as UTC
	// Seed all migration combinations before enabling the new schema.
	for i := 0; i < 6; i++ {
		var deadline any = base
		reminder := base.Add(-time.Hour)
		status := "pending"
		notified := false
		switch i {
		case 1:
			deadline = nil
		case 2:
			reminder = base.Add(time.Hour)
		case 3:
			notified = true
		case 4:
			status = "done"
		case 5:
			reminder = base.Add(-time.Hour - time.Second)
		}
		_, err = s.DB.ExecContext(ctx, `INSERT INTO tasks(user_id,title,description,deadline,reminder_at,status,is_notified) VALUES($1,$2,'',$3,$4,$5,$6)`, user, fmt.Sprintf("legacy-%d", i), deadline, reminder, status, notified)
		require.NoError(t, err)
	}
	require.NoError(t, goose.Up(s.DB, "migrations"))
	var count int
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_reminders`).Scan(&count))
	require.Equal(t, 6, count)
	var relative, absolute, sent, cancelled int
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FILTER(WHERE NOT legacy_absolute),COUNT(*) FILTER(WHERE legacy_absolute),COUNT(*) FILTER(WHERE status='sent' AND sent_at IS NULL),COUNT(*) FILTER(WHERE status='cancelled') FROM task_reminders`).Scan(&relative, &absolute, &sent, &cancelled))
	require.Equal(t, 3, absolute)
	require.Equal(t, 3, relative)
	require.Equal(t, 1, sent)
	require.Equal(t, 1, cancelled)
	// Keep imported examples out of the delivery tests.
	_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET status='cancelled'`)
	require.NoError(t, err)
	ptr := func(v int64) *int64 { return &v }
	create := func(inputs []domain.ReminderInput) int64 {
		id, err := s.SaveTask(ctx, domain.Task{UserID: user, Title: "new", Status: "pending", Deadline: &base, ReminderInputs: inputs})
		require.NoError(t, err)
		return id
	}
	t.Run("create read shift and clear atomically", func(t *testing.T) {
		id := create([]domain.ReminderInput{{OffsetMinutes: ptr(1640)}, {OffsetMinutes: ptr(0)}})
		task, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Len(t, task.Reminders, 2)
		require.True(t, task.Reminders[0].ScheduledAt.Equal(base.Add(-1640*time.Minute)))
		encoded, err := json.Marshal(task)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "reminder_at")
		require.NotContains(t, string(encoded), "is_notified")
		moved := base.Add(time.Hour)
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Deadline: &moved}))
		shifted, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.True(t, shifted.Reminders[0].ScheduledAt.Equal(task.Reminders[0].ScheduledAt.Add(time.Hour)))
		require.Error(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{ClearDeadline: true}))
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{ClearDeadline: true, Reminders: &[]domain.ReminderInput{}}))
		cleared, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Nil(t, cleared.Deadline)
		require.Empty(t, cleared.Reminders)
	})
	t.Run("limit rollback ownership and concurrent replacement", func(t *testing.T) {
		_, err := s.SaveTask(ctx, domain.Task{UserID: user, Title: "must roll back", Status: "pending", Deadline: &base, ReminderInputs: []domain.ReminderInput{{OffsetMinutes: ptr(0)}, {OffsetMinutes: ptr(1)}, {OffsetMinutes: ptr(2)}}})
		require.Error(t, err)
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE title='must roll back'`).Scan(&count))
		require.Zero(t, count)
		id := create(nil)
		other := create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}})
		foreign, err := s.GetTask(ctx, user, other)
		require.NoError(t, err)
		inputs := []domain.ReminderInput{{ID: foreign.Reminders[0].ID, OffsetMinutes: ptr(0)}}
		require.Error(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Reminders: &inputs}))
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := int64(1); i <= 8; i++ {
			wg.Add(1)
			go func(n int64) {
				defer wg.Done()
				rows := []domain.ReminderInput{{OffsetMinutes: ptr(n)}}
				errs <- s.UpdateTask(ctx, user, id, domain.TaskUpdate{Reminders: &rows})
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		task, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Len(t, task.Reminders, 1)
	})
	t.Run("premium downgrade preserves existing and sent", func(t *testing.T) {
		require.NoError(t, s.SetPremiumStatus(ctx, user, true))
		id := create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}, {OffsetMinutes: ptr(1)}, {OffsetMinutes: ptr(2)}})
		require.NoError(t, s.SetPremiumStatus(ctx, user, false))
		title := "Still editable"
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Title: &title}))
		task, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		rows := []domain.ReminderInput{}
		for _, r := range task.Reminders {
			rows = append(rows, domain.ReminderInput{ID: r.ID, OffsetMinutes: r.OffsetMinutes})
		}
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Reminders: &rows}))
		moved := base.Add(time.Hour)
		require.Error(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Deadline: &moved}))
		rows = rows[:2]
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Reminders: &rows}))
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET status='sent' WHERE id=$1`, rows[0].ID)
		require.NoError(t, err)
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Deadline: &moved}))
		task, err = s.GetTask(ctx, user, id)
		require.NoError(t, err)
		for _, r := range task.Reminders {
			if r.ID == rows[0].ID {
				require.True(t, r.ScheduledAt.Equal(base.Add(-2*time.Minute)))
			}
		}
	})
	t.Run("delivery lease retry failure and cancellation", func(t *testing.T) {
		store, err := queue.New(dsn)
		require.NoError(t, err)
		defer store.Close()
		// Only this task is due.
		id := create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}})
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET scheduled_at=CURRENT_TIMESTAMP-INTERVAL '1 minute',next_attempt_at=CURRENT_TIMESTAMP-INTERVAL '1 minute' WHERE task_id=$1`, id)
		require.NoError(t, err)
		d, err := store.ClaimReminder(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		require.Equal(t, id, d.Task.ID)
		ready, err := store.ReminderReady(ctx, *d)
		require.NoError(t, err)
		require.True(t, ready)
		second, err := store.ClaimReminder(ctx)
		require.NoError(t, err)
		require.Nil(t, second)
		require.NoError(t, store.FinishReminder(ctx, *d, errors.New("SMTP unavailable")))
		var delay float64
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM (next_attempt_at-CURRENT_TIMESTAMP)) FROM task_reminders WHERE id=$1`, d.ID).Scan(&delay))
		require.InDelta(t, 60, delay, 2)
		for attempt := 2; attempt <= 5; attempt++ {
			_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET next_attempt_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, d.ID)
			require.NoError(t, err)
			d, err = store.ClaimReminder(ctx)
			require.NoError(t, err)
			require.NotNil(t, d)
			require.Equal(t, attempt, d.Attempt)
			require.NoError(t, store.FinishReminder(ctx, *d, errors.New("SMTP unavailable")))
		}
		task, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Equal(t, "failed", task.Reminders[0].Status)
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM reminder_delivery_attempts WHERE reminder_id=$1`, d.ID).Scan(&count))
		require.Equal(t, 5, count)
		// A different task succeeds; stale acknowledgements cannot undo success.
		id = create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}})
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET next_attempt_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE task_id=$1`, id)
		require.NoError(t, err)
		d, err = store.ClaimReminder(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		old := *d
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, d.ID)
		require.NoError(t, err)
		d, err = store.ClaimReminder(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		require.NotEqual(t, old.LeaseToken, d.LeaseToken)
		require.NoError(t, store.FinishReminder(ctx, old, nil))
		ready, err = store.ReminderReady(ctx, *d)
		require.NoError(t, err)
		require.True(t, ready)
		require.NoError(t, store.FinishReminder(ctx, *d, nil))
		task, err = s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Equal(t, "sent", task.Reminders[0].Status)
		require.NotNil(t, task.Reminders[0].SentAt)
		id = create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}})
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET next_attempt_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE task_id=$1`, id)
		require.NoError(t, err)
		d, err = store.ClaimReminder(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		status := "done"
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Status: &status}))
		ready, err = store.ReminderReady(ctx, *d)
		require.NoError(t, err)
		require.False(t, ready)
		require.NoError(t, store.FinishReminder(ctx, *d, nil))
		status = "pending"
		require.NoError(t, s.UpdateTask(ctx, user, id, domain.TaskUpdate{Status: &status}))
		task, err = s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Empty(t, task.Reminders)
		next, err := store.NextReminderAt(ctx)
		require.NoError(t, err)
		require.NotNil(t, next)
	})
	t.Run("concurrent workers and expired final lease", func(t *testing.T) {
		store, err := queue.New(dsn)
		require.NoError(t, err)
		defer store.Close()
		id := create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}})
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET next_attempt_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE task_id=$1`, id)
		require.NoError(t, err)
		var wg sync.WaitGroup
		claims := make(chan int64, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d, err := store.ClaimReminder(ctx)
				errs <- err
				if d != nil {
					claims <- d.ID
				}
			}()
		}
		wg.Wait()
		close(claims)
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		require.Len(t, claims, 1)
		reminder := <-claims
		_, err = s.DB.ExecContext(ctx, `UPDATE task_reminders SET attempts=5,lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, reminder)
		require.NoError(t, err)
		d, err := store.ClaimReminder(ctx)
		require.NoError(t, err)
		require.Nil(t, d)
		task, err := s.GetTask(ctx, user, id)
		require.NoError(t, err)
		require.Equal(t, "failed", task.Reminders[0].Status)
	})
	t.Run("bulk focus burn and delete cancel delivery", func(t *testing.T) {
		for _, action := range []string{"bulk", "focus", "burn", "delete"} {
			t.Run(action, func(t *testing.T) {
				id := create([]domain.ReminderInput{{OffsetMinutes: ptr(0)}})
				switch action {
				case "bulk":
					_, err = s.BulkCompleteTasks(ctx, user, []int64{id})
				case "burn":
					status := "burnt"
					err = s.UpdateTask(ctx, user, id, domain.TaskUpdate{Status: &status})
				case "delete":
					err = s.DeleteTask(ctx, user, id)
				case "focus":
					var session int64
					err = s.DB.QueryRowContext(ctx, `INSERT INTO pomodoro_sessions(user_id,task_id,status,started_at,duration_minutes) VALUES($1,$2,'active',CURRENT_TIMESTAMP-INTERVAL '25 minutes',25) RETURNING id`, user, id).Scan(&session)
					require.NoError(t, err)
					err = s.FinishPomodoroSession(ctx, user, session, "completed", true)
				}
				require.NoError(t, err)
				require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_reminders WHERE task_id=$1 AND status='scheduled'`, id).Scan(&count))
				require.Zero(t, count)
			})
		}
	})

}
