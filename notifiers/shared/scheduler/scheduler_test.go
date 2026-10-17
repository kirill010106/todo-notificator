package scheduler

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeQueue struct {
	deliveries                             []domain.ReminderDelivery
	claimErr, readyErr, finishErr, nextErr error
	ready                                  bool
	finished                               []error
	next                                   *time.Time
	claims                                 int
}

func (f *fakeQueue) ClaimReminder(context.Context) (*domain.ReminderDelivery, error) {
	f.claims++
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if len(f.deliveries) == 0 {
		return nil, nil
	}
	d := f.deliveries[0]
	f.deliveries = f.deliveries[1:]
	return &d, nil
}
func (f *fakeQueue) ReminderReady(context.Context, domain.ReminderDelivery) (bool, error) {
	return f.ready, f.readyErr
}
func (f *fakeQueue) FinishReminder(_ context.Context, _ domain.ReminderDelivery, err error) error {
	f.finished = append(f.finished, err)
	return f.finishErr
}
func (f *fakeQueue) NextReminderAt(context.Context) (*time.Time, error) { return f.next, f.nextErr }

type fakeSender struct {
	calls    int
	interval time.Duration
	err      error
	cancel   context.CancelFunc
}

func (f *fakeSender) Send(_ domain.User, _ domain.Task, d time.Duration) error {
	f.calls++
	f.interval = d
	if f.cancel != nil {
		f.cancel()
	}
	return f.err
}
func TestReminderScheduler(t *testing.T) {
	offset := int64(1640)
	delivery := domain.ReminderDelivery{ID: 1, Attempt: 1, OffsetMinutes: &offset}
	for _, tt := range []struct {
		name                       string
		claim, ready, finish, send error
		eligible                   bool
		want                       int
	}{
		{name: "success", eligible: true, want: 1}, {name: "send failure acknowledged", eligible: true, send: errors.New("smtp"), want: 1},
		{name: "cancelled before send"}, {name: "claim failure", claim: errors.New("db")}, {name: "preflight failure", ready: errors.New("db")},
		{name: "acknowledgement failure", eligible: true, finish: errors.New("db"), want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeQueue{deliveries: []domain.ReminderDelivery{delivery}, claimErr: tt.claim, readyErr: tt.ready, finishErr: tt.finish, ready: tt.eligible}
			sender := &fakeSender{err: tt.send}
			s := New(slog.New(slog.NewTextHandler(io.Discard, nil)), q, sender)
			s.poll(context.Background())
			require.Equal(t, tt.want, sender.calls)
			if tt.want > 0 {
				require.Equal(t, 1640*time.Minute, sender.interval)
				require.Len(t, q.finished, 1)
				require.Equal(t, tt.send, q.finished[0])
			}
		})
	}
	t.Run("shutdown during SMTP still acknowledges", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		q := &fakeQueue{deliveries: []domain.ReminderDelivery{delivery}, ready: true}
		sender := &fakeSender{cancel: cancel}
		s := New(slog.Default(), q, sender)
		s.Reschedule()
		s.Reschedule()
		s.Start(ctx)
		require.Len(t, q.finished, 1)
	})
	t.Run("stopped scheduler exits", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := New(slog.Default(), &fakeQueue{}, &fakeSender{})
		s.Start(ctx)
	})
}
