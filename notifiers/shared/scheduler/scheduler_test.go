package scheduler_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/kirill010106/todo-notificator/notifiers/shared/scheduler"
	"github.com/stretchr/testify/assert"
)

// mockStorage implements storage.Storage for tests
type mockStorage struct {
	tasks               []domain.TaskWithUser
	getTasksErr         error
	markedAsNotified    []int64
	markedSet           map[int64]struct{}
	markTaskErr         error
	nearestReminderAt   *time.Time
	nearestReminderErr  error
}

func (m *mockStorage) GetPendingTasksWithUsers(ctx context.Context) ([]domain.TaskWithUser, error) {
	if m.getTasksErr != nil {
		return nil, m.getTasksErr
	}

	if len(m.tasks) == 0 {
		return nil, nil
	}

	pending := make([]domain.TaskWithUser, 0, len(m.tasks))
	for _, item := range m.tasks {
		if m.markedSet != nil {
			if _, ok := m.markedSet[item.Task.ID]; ok {
				continue
			}
		}
		pending = append(pending, item)
	}

	return pending, nil
}

func (m *mockStorage) GetTasksDueBetween(ctx context.Context, from, to time.Time) ([]domain.Task, error) {
	// Not used in poll, so not implemented
	return nil, nil
}

func (m *mockStorage) GetNearestPendingReminderAt(ctx context.Context) (*time.Time, error) {
	if m.nearestReminderErr != nil {
		return nil, m.nearestReminderErr
	}

	if len(m.tasks) > 0 {
		hasPending := false
		for _, item := range m.tasks {
			if m.markedSet != nil {
				if _, ok := m.markedSet[item.Task.ID]; ok {
					continue
				}
			}
			hasPending = true
			break
		}

		if !hasPending {
			return nil, nil
		}
	}

	return m.nearestReminderAt, nil
}

func (m *mockStorage) MarkTaskAsNotified(ctx context.Context, taskID int64) error {
	if m.markTaskErr != nil {
		return m.markTaskErr
	}
	if m.markedAsNotified == nil {
		m.markedAsNotified = make([]int64, 0)
	}
	if m.markedSet == nil {
		m.markedSet = make(map[int64]struct{})
	}
	m.markedSet[taskID] = struct{}{}
	m.markedAsNotified = append(m.markedAsNotified, taskID)
	return nil
}

// mockSender implements Sender for tests
type mockSender struct {
	sentTasks []domain.Task
	sendErr   error
	sentCh    chan struct{}
}

func (m *mockSender) Send(user domain.User, task domain.Task, interval time.Duration) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	if m.sentTasks == nil {
		m.sentTasks = make([]domain.Task, 0)
	}
	m.sentTasks = append(m.sentTasks, task)

	if m.sentCh != nil {
		select {
		case m.sentCh <- struct{}{}:
		default:
		}
	}

	return nil
}

func TestScheduler_Poll(t *testing.T) {
	// Create logger that will not flood console
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	
	now := time.Now()
	intervals := []time.Duration{1 * time.Hour}

	testCases := []struct {
		name               string
		tasks              []domain.TaskWithUser
		getTasksErr        error
		sendErr            error
		markTaskErr        error
		expectedSentCount  int
		expectedMarkedTask []int64
	}{
		{
			name: "Successful sending of burning task",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 1, Email: "test@test.com"},
					Task: domain.Task{
						ID:       1,
						Title:    "Burning task",
						ReminderAt: func() *time.Time { t := now; return &t }(), // Deadline is now
					},
				},
			},
			expectedSentCount:  1,
			expectedMarkedTask: []int64{1},
		},
		{
			name: "Task with deadline in future (should not be sent)",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 2, Email: "test@test.com"},
					Task: domain.Task{
						ID:       2,
						Title:    "Task in future",
						ReminderAt: func() *time.Time { t := now.Add(2 * time.Hour); return &t }(), // Deadline in 2 hours, interval 1 hour - send early
					},
				},
			},
			expectedSentCount:  0,
			expectedMarkedTask: nil,
		},
		{
			name: "Error during sending (task not marked)",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 3, Email: "test@test.com"},
					Task: domain.Task{
						ID:       3,
						Title:    "Task with sending error",
						ReminderAt: func() *time.Time { t := now; return &t }(),
					},
				},
			},
			sendErr:            errors.New("SMTP error"),
			expectedSentCount:  0,
			expectedMarkedTask: nil,
		},
		{
			name: "Task without deadline (skipped)",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 4, Email: "test@test.com"},
					Task: domain.Task{
						ID:       4,
						Title:    "Without deadline",
						ReminderAt: nil,
					},
				},
			},
			expectedSentCount:  0,
			expectedMarkedTask: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			storageMock := &mockStorage{
				tasks:       tc.tasks,
				getTasksErr: tc.getTasksErr,
				markTaskErr: tc.markTaskErr,
			}
			senderMock := &mockSender{
				sendErr: tc.sendErr,
			}

			// Instead of exporting Poll method, we can just put helper-function in scheduler package or create alias.
			// But since s.poll() is private, we will use the fact that it is called by reschedule trigger via channel
			
			// Create scheduler
			s := scheduler.New(log, storageMock, senderMock, intervals)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			
			// Start scheduler in background - it will call s.poll(ctx) on start!
			go s.Start(ctx)

			// Give time to execute poll and finish goroutine on timeout
			time.Sleep(50 * time.Millisecond)
			cancel() // forcefully stop

			assert.Len(t, senderMock.sentTasks, tc.expectedSentCount, "Sent tasks count mismatch")
			assert.Equal(t, tc.expectedMarkedTask, storageMock.markedAsNotified, "Marked tasks mismatch")
		})
	}
}

func TestScheduler_RescheduleTriggersSendAtReminderTime(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	reminderAt := time.Now().Add(900 * time.Millisecond)

	storageMock := &mockStorage{}
	senderMock := &mockSender{sentCh: make(chan struct{}, 1)}

	task := domain.TaskWithUser{
		User: domain.User{ID: 1, Email: "test@test.com"},
		Task: domain.Task{
			ID:         10,
			Title:      "scheduled task",
			ReminderAt: &reminderAt,
		},
	}

	storageMock.tasks = []domain.TaskWithUser{task}
	storageMock.nearestReminderAt = &reminderAt

	s := scheduler.New(log, storageMock, senderMock, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go s.Start(ctx)

	// Trigger a refresh after webhook-like event.
	s.Reschedule()

	select {
	case <-senderMock.sentCh:
	case <-time.After(2 * time.Second):
		t.Fatal("task was not sent at reminder time after reschedule")
	}

	assert.Len(t, senderMock.sentTasks, 1, "task should be sent around reminder time without waiting 5m ticker")
	assert.Equal(t, []int64{10}, storageMock.markedAsNotified)
}

