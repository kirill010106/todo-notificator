package verification

import (
	"context"
	"log/slog"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

type Store interface {
	ClaimVerificationDelivery(context.Context) (*domain.VerificationDelivery, error)
	FinishVerificationDelivery(context.Context, domain.VerificationDelivery, error) error
}

type Sender interface {
	SendVerificationEmail(string, string) error
}

type Worker struct {
	log    *slog.Logger
	store  Store
	sender Sender
}

func New(log *slog.Logger, store Store, sender Sender) *Worker {
	return &Worker{log: log, store: store, sender: sender}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		w.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		delivery, err := w.store.ClaimVerificationDelivery(claimCtx)
		cancel()
		if err != nil {
			w.log.Error("cannot claim verification email", "error", err)
			return
		}
		if delivery == nil {
			return
		}
		sendErr := w.sender.SendVerificationEmail(delivery.Email, delivery.Token)
		if sendErr != nil {
			w.log.Error("verification email delivery failed", "delivery_id", delivery.ID, "attempt", delivery.Attempt, "error", sendErr)
		}
		ackCtx, ackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = w.store.FinishVerificationDelivery(ackCtx, *delivery, sendErr)
		ackCancel()
		if err != nil {
			w.log.Error("cannot acknowledge verification email", "delivery_id", delivery.ID, "error", err)
			return
		}
	}
}
