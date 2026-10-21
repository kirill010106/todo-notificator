//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	todonotificator "github.com/kirill010106/todo-notificator"
	paymentwebhook "github.com/kirill010106/todo-notificator/internal/http-server/handlers/payments/webhook"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/pressly/goose/v3"
	payment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type paymentFinderFunc func(context.Context, string) (*payment.Payment, error)

func (f paymentFinderFunc) FindPayment(ctx context.Context, id string) (*payment.Payment, error) {
	return f(ctx, id)
}

func TestPaymentTerminalSecurityIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("payment_security"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	s, err := postgres.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(s.DB, "migrations"))
	sequence := 0
	create := func(t *testing.T) (int64, string) {
		t.Helper()
		sequence++
		user, err := s.SaveUser(ctx, fmt.Sprintf("payment-%d@test.local", sequence), []byte("hash"))
		require.NoError(t, err)
		id := fmt.Sprintf("payment-%d", sequence)
		_, err = s.CreatePayment(ctx, id, user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		return user, id
	}
	state := func(t *testing.T, id string) (string, bool, time.Time) {
		t.Helper()
		var status string
		var premium bool
		var updated time.Time
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT p.status,u.is_premium,p.updated_at FROM payments p JOIN users u ON u.id=p.user_id WHERE p.yookassa_payment_id=$1`, id).Scan(&status, &premium, &updated))
		return status, premium, updated
	}
	assertState := func(t *testing.T, id, expected string, premium bool) time.Time {
		t.Helper()
		status, actualPremium, stamp := state(t, id)
		require.Equal(t, expected, status)
		require.Equal(t, premium, actualPremium)
		return stamp
	}
	fire := func(t *testing.T, id, event string, provider payment.Status, want int) {
		t.Helper()
		var lookups atomic.Int32
		finder := paymentFinderFunc(func(_ context.Context, requested string) (*payment.Payment, error) {
			lookups.Add(1)
			require.Equal(t, id, requested)
			return &payment.Payment{ID: id, Status: provider}, nil
		})
		body, err := json.Marshal(map[string]any{"event": "payment." + event, "object": map[string]string{"id": id, "status": event}})
		require.NoError(t, err)
		r := httptest.NewRequest("POST", "/webhooks/yookassa", bytes.NewReader(body))
		r.RemoteAddr = "185.71.76.10:443"
		w := httptest.NewRecorder()
		paymentwebhook.New(slog.New(slog.DiscardHandler), s, finder)(w, r)
		require.Equal(t, want, w.Code, w.Body.String())
		require.Equal(t, int32(1), lookups.Load())
	}

	t.Run("unconfirmed cancellation cannot overwrite successful payment", func(t *testing.T) {
		_, id := create(t)
		_, err := s.CompletePaymentAndGrantPremium(ctx, id)
		require.NoError(t, err)
		before := assertState(t, id, "succeeded", true)
		for _, provider := range []payment.Status{payment.Pending, payment.Succeeded} {
			fire(t, id, "canceled", provider, 400)
		}
		require.True(t, before.Equal(assertState(t, id, "succeeded", true)))
	})
	t.Run("canceled is idempotent and cannot later grant Premium", func(t *testing.T) {
		_, id := create(t)
		fire(t, id, "canceled", payment.Canceled, 200)
		stamp := assertState(t, id, "canceled", false)
		fire(t, id, "canceled", payment.Canceled, 200)
		fire(t, id, "succeeded", payment.Succeeded, 200) // Contradictory verified event is ignored and logged.
		require.True(t, stamp.Equal(assertState(t, id, "canceled", false)))
	})
	t.Run("succeeded is idempotent and cannot be canceled", func(t *testing.T) {
		_, id := create(t)
		fire(t, id, "succeeded", payment.Succeeded, 200)
		stamp := assertState(t, id, "succeeded", true)
		fire(t, id, "succeeded", payment.Succeeded, 200)
		fire(t, id, "canceled", payment.Canceled, 200)
		require.True(t, stamp.Equal(assertState(t, id, "succeeded", true)))
		_, err := s.UpdatePaymentStatus(ctx, id, "canceled")
		require.ErrorIs(t, err, storage.ErrPaymentStateConflict)
	})
	t.Run("opposing concurrent terminal transitions remain consistent", func(t *testing.T) {
		_, id := create(t)
		start := make(chan struct{})
		results := make(chan error, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				var err error
				if i%2 == 0 {
					_, err = s.CompletePaymentAndGrantPremium(ctx, id)
				} else {
					_, err = s.UpdatePaymentStatus(ctx, id, "canceled")
				}
				results <- err
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, storage.ErrPaymentStateConflict)
			}
		}
		require.Equal(t, 8, successes)
		status, premium, _ := state(t, id)
		require.Contains(t, []string{"succeeded", "canceled"}, status)
		require.Equal(t, status == "succeeded", premium)
	})
	t.Run("failed Premium write rolls payment status back", func(t *testing.T) {
		user, id := create(t)
		_, err := s.DB.ExecContext(ctx, fmt.Sprintf(`CREATE FUNCTION reject_test_premium() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id=%d AND NEW.is_premium THEN RAISE EXCEPTION 'injected Premium failure'; END IF; RETURN NEW; END; $$`, user))
		require.NoError(t, err)
		_, err = s.DB.ExecContext(ctx, `CREATE TRIGGER reject_test_premium BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION reject_test_premium()`)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := s.DB.ExecContext(context.Background(), `DROP TRIGGER reject_test_premium ON users`)
			require.NoError(t, err)
			_, err = s.DB.ExecContext(context.Background(), `DROP FUNCTION reject_test_premium()`)
			require.NoError(t, err)
		})
		_, err = s.CompletePaymentAndGrantPremium(ctx, id)
		require.ErrorContains(t, err, "injected Premium failure")
		fire(t, id, "succeeded", payment.Succeeded, 500)
		assertState(t, id, "pending", false)
	})
	t.Run("missing record stays retryable", func(t *testing.T) {
		_, err := s.UpdatePaymentStatus(ctx, "missing", "canceled")
		require.ErrorIs(t, err, storage.ErrPaymentNotFound)
		_, err = s.CompletePaymentAndGrantPremium(ctx, "missing")
		require.ErrorIs(t, err, storage.ErrPaymentNotFound)
	})
}
