package scheduler

import (
	"context"
	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/kirill010106/todo-notificator/notifiers/shared/storage"
	"log/slog"
	"time"
)

type Sender interface {
	Send(domain.User, domain.Task, time.Duration) error
}
type Scheduler struct {
	log        *slog.Logger
	storage    storage.Storage
	sender     Sender
	reschedule chan struct{}
}

// Reminder intervals belong to each database record.
func New(log *slog.Logger, store storage.Storage, sender Sender) *Scheduler {
	return &Scheduler{log: log, storage: store, sender: sender, reschedule: make(chan struct{}, 1)}
}
func (s *Scheduler) Reschedule() {
	select {
	case s.reschedule <- struct{}{}:
	default:
	}
}
func (s *Scheduler) Start(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reschedule:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		s.poll(ctx)
		delay := 30 * time.Second
		if next, err := s.storage.NextReminderAt(ctx); err != nil {
			s.log.Error("cannot load next reminder", "error", err)
		} else if next != nil {
			until := time.Until(*next)
			if until < delay {
				delay = until
			}
			if delay < 250*time.Millisecond {
				delay = 250 * time.Millisecond
			}
		}
		timer.Reset(delay)
	}
}
func (s *Scheduler) poll(ctx context.Context) {
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		op, cancel := context.WithTimeout(ctx, 5*time.Second)
		delivery, err := s.storage.ClaimReminder(op)
		cancel()
		if err != nil {
			s.log.Error("cannot claim reminder", "error", err)
			return
		}
		if delivery == nil {
			return
		}
		op, cancel = context.WithTimeout(ctx, 5*time.Second)
		ready, err := s.storage.ReminderReady(op, *delivery)
		cancel()
		if err != nil {
			s.log.Error("cannot check reminder lease", "reminder_id", delivery.ID, "error", err)
			return
		}
		if !ready {
			continue
		}
		interval := time.Duration(0)
		if delivery.OffsetMinutes != nil {
			interval = time.Duration(*delivery.OffsetMinutes) * time.Minute
		}
		sendErr := s.sender.Send(delivery.User, delivery.Task, interval)
		if sendErr != nil {
			s.log.Error("reminder delivery failed", "reminder_id", delivery.ID, "attempt", delivery.Attempt, "error", sendErr)
		}
		// Persist acknowledgement even when shutdown arrived during the SMTP call.
		op, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		err = s.storage.FinishReminder(op, *delivery, sendErr)
		cancel()
		if err != nil {
			s.log.Error("cannot acknowledge reminder", "reminder_id", delivery.ID, "error", err)
			return
		}
	}
}
