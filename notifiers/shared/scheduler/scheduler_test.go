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

// mockStorage имплементирует storage.Storage для тестов
type mockStorage struct {
	tasks               []domain.TaskWithUser
	getTasksErr         error
	markedAsNotified    []int64
	markTaskErr         error
}

func (m *mockStorage) GetPendingTasksWithUsers(ctx context.Context) ([]domain.TaskWithUser, error) {
	if m.getTasksErr != nil {
		return nil, m.getTasksErr
	}
	return m.tasks, nil
}

func (m *mockStorage) GetTasksDueBetween(ctx context.Context, from, to time.Time) ([]domain.Task, error) {
	// Не используется в poll, так что не реализовываем
	return nil, nil
}

func (m *mockStorage) MarkTaskAsNotified(ctx context.Context, taskID int64) error {
	if m.markTaskErr != nil {
		return m.markTaskErr
	}
	if m.markedAsNotified == nil {
		m.markedAsNotified = make([]int64, 0)
	}
	m.markedAsNotified = append(m.markedAsNotified, taskID)
	return nil
}

// mockSender имплементирует Sender для тестов
type mockSender struct {
	sentTasks []domain.Task
	sendErr   error
}

func (m *mockSender) Send(user domain.User, task domain.Task, interval time.Duration) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	if m.sentTasks == nil {
		m.sentTasks = make([]domain.Task, 0)
	}
	m.sentTasks = append(m.sentTasks, task)
	return nil
}

func TestScheduler_Poll(t *testing.T) {
	// Создаем логгер, который не будет флудить в консоль
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
			name: "Успешная отправка горящей задачи",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 1, Email: "test@test.com"},
					Task: domain.Task{
						ID:       1,
						Title:    "Горит задача",
						ReminderAt: func() *time.Time { t := now; return &t }(), // Дедлайн сейчас
					},
				},
			},
			expectedSentCount:  1,
			expectedMarkedTask: []int64{1},
		},
		{
			name: "Задача с дедлайном в будущем (не должна отправляться)",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 2, Email: "test@test.com"},
					Task: domain.Task{
						ID:       2,
						Title:    "Задача в будущем",
						ReminderAt: func() *time.Time { t := now.Add(2 * time.Hour); return &t }(), // Дедлайн через 2 часа, интервал 1 час — отправлять рано
					},
				},
			},
			expectedSentCount:  0,
			expectedMarkedTask: nil,
		},
		{
			name: "Ошибка при отправке (задача не помечается)",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 3, Email: "test@test.com"},
					Task: domain.Task{
						ID:       3,
						Title:    "Задача с ошибкой отправки",
						ReminderAt: func() *time.Time { t := now; return &t }(),
					},
				},
			},
			sendErr:            errors.New("SMTP error"),
			expectedSentCount:  0,
			expectedMarkedTask: nil,
		},
		{
			name: "Задача без дедлайна (пропускается)",
			tasks: []domain.TaskWithUser{
				{
					User: domain.User{ID: 4, Email: "test@test.com"},
					Task: domain.Task{
						ID:       4,
						Title:    "Без дедлайна",
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

			// Вместо экспорта метода Poll, мы можем просто положить в пакет scheduler функцию-хелпер для теста или создать алиас.
			// Но так как функция s.poll() приватная, мы воспользуемся тем, что вызываем ее через триггер reschedule каналом
			
			// Создаем шедулер
			s := scheduler.New(log, storageMock, senderMock, intervals)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			
			// Запускаем шедулер в фоне - он вызовет s.poll(ctx) при старте!
			go s.Start(ctx)

			// Даем время на выполнение poll и завершение горутины по таймауту
			time.Sleep(50 * time.Millisecond)
			cancel() // принудительно останавливаем

			assert.Len(t, senderMock.sentTasks, tc.expectedSentCount, "Количество отправленных задач не совпадает")
			assert.Equal(t, tc.expectedMarkedTask, storageMock.markedAsNotified, "Отмеченные задачи не совпадают")
		})
	}
}

