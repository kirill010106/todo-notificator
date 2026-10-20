package passwordreset

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"github.com/stretchr/testify/require"
)

type storeStub struct {
	delivery *domain.VerificationDelivery
	finished []error
}

func (s *storeStub) ClaimPasswordResetDelivery(context.Context) (*domain.VerificationDelivery, error) {
	d := s.delivery
	s.delivery = nil
	return d, nil
}
func (s *storeStub) FinishPasswordResetDelivery(_ context.Context, _ domain.VerificationDelivery, err error) error {
	s.finished = append(s.finished, err)
	return nil
}

type senderStub struct{ err error }

func (s senderStub) SendPasswordResetEmail(email, token string) error {
	if email != "test@example.com" || token != "secret-link-token" {
		return errors.New("unexpected reset payload")
	}
	return s.err
}

func TestPollAcknowledgesDeliveryResultForRetry(t *testing.T) {
	store := &storeStub{delivery: &domain.VerificationDelivery{ID: 8, Attempt: 1, Email: "test@example.com", Token: "secret-link-token"}}
	w := New(slog.New(slog.DiscardHandler), store, senderStub{err: errors.New("smtp unavailable")})
	w.poll(context.Background())
	require.Len(t, store.finished, 1)
	require.EqualError(t, store.finished[0], "smtp unavailable")
}

func TestPollAcknowledgesSuccessfulDelivery(t *testing.T) {
	store := &storeStub{delivery: &domain.VerificationDelivery{ID: 8, Attempt: 1, Email: "test@example.com", Token: "secret-link-token"}}
	w := New(slog.New(slog.DiscardHandler), store, senderStub{})
	w.poll(context.Background())
	require.Len(t, store.finished, 1)
	require.NoError(t, store.finished[0])
}
