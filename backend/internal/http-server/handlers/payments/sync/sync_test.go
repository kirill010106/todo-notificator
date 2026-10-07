package sync

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockPaymentStatusChecker struct {
	mock.Mock
}

func (m *mockPaymentStatusChecker) GetLatestPendingPayment(ctx context.Context, userID int64) (string, error) {
	args := m.Called(ctx, userID)
	return args.String(0), args.Error(1)
}

func (m *mockPaymentStatusChecker) UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error) {
	args := m.Called(ctx, yookassaID, status)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockPaymentStatusChecker) GrantPremium(ctx context.Context, userID int64) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *mockPaymentStatusChecker) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

type mockPaymentFinder struct {
	mock.Mock
}

func (m *mockPaymentFinder) FindPayment(ctx context.Context, id string) (*yoopayment.Payment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*yoopayment.Payment), args.Error(1)
}

func TestSyncPayment_AlreadyPremium(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := new(mockPaymentStatusChecker)
	finder := new(mockPaymentFinder)

	checker.On("GetUserByID", mock.Anything, int64(42)).Return(&domain.User{
		ID:        42,
		IsPremium: true,
	}, nil)

	h := New(log, checker, finder)
	mw := auth.New(secret)
	wrappedHandler := mw(h)

	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "u@t.com", IsPremium: true}, secret, time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/payments/sync", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"is_premium":true`)
}

func TestSyncPayment_SucceededInGateway(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := new(mockPaymentStatusChecker)
	finder := new(mockPaymentFinder)

	checker.On("GetUserByID", mock.Anything, int64(42)).Return(&domain.User{
		ID:        42,
		IsPremium: false,
	}, nil)
	checker.On("GetLatestPendingPayment", mock.Anything, int64(42)).Return("yoo-pay-1", nil)
	finder.On("FindPayment", mock.Anything, "yoo-pay-1").Return(&yoopayment.Payment{
		ID:     "yoo-pay-1",
		Status: yoopayment.Succeeded,
	}, nil)
	checker.On("UpdatePaymentStatus", mock.Anything, "yoo-pay-1", "succeeded").Return(int64(42), nil)
	checker.On("GrantPremium", mock.Anything, int64(42)).Return(nil)

	h := New(log, checker, finder)
	mw := auth.New(secret)
	wrappedHandler := mw(h)

	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "u@t.com", IsPremium: false}, secret, time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/payments/sync", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"is_premium":true`)
	require.Contains(t, w.Body.String(), `"payment_status":"succeeded"`)

	checker.AssertCalled(t, "GrantPremium", mock.Anything, int64(42))
}
