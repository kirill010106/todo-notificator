package sync

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	payment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"
)

type statusStub struct {
	pending             string
	pendingIDs          []string
	lookupErr, writeErr error
	grants, updates     int
}

func (s *statusStub) GetUserByID(context.Context, int64) (*domain.User, error) {
	return &domain.User{ID: 42}, nil
}
func (s *statusStub) GetPendingPayments(context.Context, int64) ([]string, error) {
	if s.lookupErr != nil {
		return nil, s.lookupErr
	}
	if s.pendingIDs != nil {
		return s.pendingIDs, nil
	}
	if s.pending == "" {
		return []string{}, nil
	}
	return []string{s.pending}, nil
}
func (s *statusStub) CompletePaymentAndGrantPremium(context.Context, string) (int64, error) {
	s.grants++
	return 42, s.writeErr
}
func (s *statusStub) UpdatePaymentStatus(context.Context, string, string) (int64, error) {
	s.updates++
	return 42, s.writeErr
}

type finderStub struct {
	payment *payment.Payment
	err     error
}

func (f finderStub) FindPayment(context.Context, string) (*payment.Payment, error) {
	return f.payment, f.err
}
func TestSyncDoesNotGrantPremiumWithoutConfirmedPayment(t *testing.T) {
	for _, tc := range []struct {
		name, pending, status           string
		lookupErr, gatewayErr, writeErr error
		nilFinder, missing              bool
		want, grants, updates           int
	}{
		{name: "no pending payment", want: 200},
		{name: "pending lookup failed", lookupErr: errors.New("db"), want: 500},
		{name: "gateway unavailable", pending: "id", nilFinder: true, want: 200},
		{name: "gateway failed", pending: "id", gatewayErr: errors.New("network"), want: 502},
		{name: "payment missing", pending: "id", missing: true, want: 200},
		{name: "still pending", pending: "id", status: "pending", want: 200},
		{name: "canceled", pending: "id", status: "canceled", want: 200, updates: 1},
		{name: "cancel write failed", pending: "id", status: "canceled", writeErr: errors.New("db"), want: 500, updates: 1},
		{name: "premium write failed", pending: "id", status: "succeeded", writeErr: errors.New("db"), want: 500, grants: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := &statusStub{pending: tc.pending, lookupErr: tc.lookupErr, writeErr: tc.writeErr}
			value := &payment.Payment{ID: tc.pending, Status: payment.Status(tc.status)}
			if tc.missing {
				value = nil
			}
			var finder PaymentFinder = finderStub{value, tc.gatewayErr}
			if tc.nilFinder {
				finder = nil
			}
			token, err := jwt.NewAccessToken(domain.User{ID: 42}, "jwt", time.Hour)
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/payments/sync", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			auth.New("jwt")(New(slog.New(slog.DiscardHandler), checker, finder)).ServeHTTP(res, req)
			require.Equal(t, tc.want, res.Code, res.Body.String())
			require.Equal(t, tc.grants, checker.grants)
			require.Equal(t, tc.updates, checker.updates)
			var body map[string]any
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
			require.NotEqual(t, true, body["is_premium"])
		})
	}
}
