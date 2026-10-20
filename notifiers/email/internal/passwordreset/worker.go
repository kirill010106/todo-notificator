package passwordreset

import (
	"context"
	"log/slog"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

type Store interface {
	ClaimPasswordResetDelivery(context.Context) (*domain.VerificationDelivery, error)
	FinishPasswordResetDelivery(context.Context, domain.VerificationDelivery, error) error
}

type Sender interface {
	SendPasswordResetEmail(string, string) error
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
		delivery, err := w.store.ClaimPasswordResetDelivery(claimCtx)
		cancel()
		if err != nil {
			w.log.Error("cannot claim password reset email", "error", err)
			return
		}
		if delivery == nil {
			return
		}
		if validator, ok := w.store.(interface {
			PasswordResetReady(context.Context, domain.VerificationDelivery) (bool, error)
		}); ok {
			readyCtx, readyCancel := context.WithTimeout(ctx, 5*time.Second)
			ready, checkErr := validator.PasswordResetReady(readyCtx, *delivery)
			readyCancel()
			if checkErr != nil {
				w.log.Error("cannot validate password reset delivery", "delivery_id", delivery.ID)
				return
			}
			if !ready {
				continue
			}
		}
		sendErr := w.sender.SendPasswordResetEmail(delivery.Email, delivery.Token)
		if sendErr != nil {
			w.log.Error("password reset email delivery failed", "delivery_id", delivery.ID, "attempt", delivery.Attempt, "error", sendErr)
		}
		ackCtx, ackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = w.store.FinishPasswordResetDelivery(ackCtx, *delivery, sendErr)
		ackCancel()
		if err != nil {
			w.log.Error("cannot acknowledge password reset email", "delivery_id", delivery.ID, "error", err)
			return
		}
	}
}
