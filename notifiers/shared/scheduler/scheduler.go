package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/kirill010106/todo-notificator/notifiers/shared/storage"
)

// realizr of sender.Sender
type Sender interface {
	Send(user domain.User, task domain.Task, interval time.Duration) error
}

type Scheduler struct {
	log       *slog.Logger
	storage   storage.Storage
	sender    Sender
	intervals []time.Duration

	mu         sync.Mutex
	timers     map[string]*time.Timer
	reschedule chan struct{}
}

func New(
	log *slog.Logger,
	storage storage.Storage,
	sender Sender,
	intervals []time.Duration,
) *Scheduler {
	return &Scheduler{
		log:        log,
		storage:    storage,
		sender:     sender,
		intervals:  intervals,
		timers:     make(map[string]*time.Timer),
		reschedule: make(chan struct{}, 1),
	}
}


func (s *Scheduler) Reschedule() {
	select {
	case s.reschedule <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.log.Info("scheduler started", slog.Any("intervals", s.intervals))

	s.reload(ctx)

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.log.Debug("ticker reload")
			s.reload(ctx)
		case <-s.reschedule:
			s.log.Info("reschedule triggered")
			s.reload(ctx)
		case <-ctx.Done():
			s.cancelAll()
			s.log.Info("scheduler stopped")
			return
		}
	}
}

func (s *Scheduler) reload(ctx context.Context) {
	tasks, err := s.storage.GetPendingTasksWithDeadline(ctx)
	if err != nil {
		s.log.Error("failed to load tasks", slog.String("error", err.Error()))
		return
	}
	s.log.Info("reloading tasks", slog.Int("count", len(tasks)))

	s.cancelAll()

	now := time.Now()

	for _, task := range tasks {
		if task.Deadline == nil {
			continue
		}

		user, err := s.storage.GetUserByID(ctx, task.UserID)
		if err != nil || user == nil {
			s.log.Warn("user not found",
				slog.Int64("user_id", task.UserID),
			)
			continue
		}

		for _, interval := range s.intervals {
			fireAt := task.Deadline.Add(-interval)

			if !fireAt.After(now) {
				continue
			}

			s.scheduleOne(ctx, task, *user, fireAt, interval)
		}
	}
}

func (s *Scheduler) scheduleOne(
	ctx context.Context,
	task domain.Task,
	user domain.User,
	fireAt time.Time,
	interval time.Duration,
) {
	key := fmt.Sprintf("%d:%s", task.ID, interval.String())
	delay := time.Until(fireAt)

	s.log.Debug("scheduling notification",
		slog.String("key", key),
		slog.String("task", task.Title),
		slog.String("to", user.Email),
		slog.Duration("in", delay),
	)

	timer := time.AfterFunc(delay, func() {
		if err := s.sender.Send(user, task, interval); err != nil {
			s.log.Error("failed to send notification",
				slog.String("key", key),
				slog.String("error", err.Error()),
			)
		}

		s.mu.Lock()
		delete(s.timers, key)
		s.mu.Unlock()

	})

	s.mu.Lock()
	s.timers[key] = timer
	s.mu.Unlock()
}

func (s *Scheduler) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, timer := range s.timers {
		timer.Stop()
		delete(s.timers, key)
	}
}
