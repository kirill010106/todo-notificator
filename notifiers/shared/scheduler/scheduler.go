package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/kirill010106/todo-notificator/notifiers/shared/storage"
)

type Sender interface {
	Send(user domain.User, task domain.Task, interval time.Duration) error
}

type Scheduler struct {
	log        *slog.Logger
	storage    storage.Storage
	sender     Sender
	reschedule chan struct{}
}

const fallbackPollInterval = 5 * time.Minute

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
	s.log.Info("polling scheduler started")

	nextDelay := s.pollAndComputeNextDelay(ctx)
	timer := time.NewTimer(nextDelay)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			nextDelay = s.pollAndComputeNextDelay(ctx)
			timer.Reset(nextDelay)
		case <-s.reschedule:
			s.log.Info("reschedule polling triggered via webhook")
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			nextDelay = s.pollAndComputeNextDelay(ctx)
			timer.Reset(nextDelay)
		case <-ctx.Done():
			s.log.Info("scheduler stopped")
			return
		}
	}
}

func (s *Scheduler) pollAndComputeNextDelay(ctx context.Context) time.Duration {
	s.poll(ctx)

	nextReminderAt, err := s.storage.GetNearestPendingReminderAt(ctx)
	if err != nil {
		s.log.Error("failed to load nearest reminder", sl.Err(err))
		return fallbackPollInterval
	}

	if nextReminderAt == nil {
		return fallbackPollInterval
	}

	delay := time.Until(*nextReminderAt)
	if delay < 0 {
		return 0
	}

	return delay
}

func (s *Scheduler) poll(ctx context.Context) {
	items, err := s.storage.GetPendingTasksWithUsers(ctx)
	if err != nil {
		s.log.Error("failed to load tasks", sl.Err(err))
		return
	}

	if len(items) > 0 {
		s.log.Info("checking tasks for notification", slog.Int("count", len(items)))
	}

	now := time.Now()

	for _, item := range items {
		if item.Task.ReminderAt == nil || item.Task.ReminderAt.After(now) {
			continue
		}

		s.log.Info("sending notification", slog.Int64("task_id", item.Task.ID), slog.String("email", item.User.Email))

		if err := s.sender.Send(item.User, item.Task, time.Duration(0)); err != nil {
			s.log.Error("failed to send notification", sl.Err(err))
			continue
		}

		if err := s.storage.MarkTaskAsNotified(ctx, item.Task.ID); err != nil {
			s.log.Error("failed to mark task as notified", slog.Int64("task_id", item.Task.ID), sl.Err(err))
		}

	}
}

