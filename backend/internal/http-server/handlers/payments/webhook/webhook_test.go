package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/domain"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	yoowebhook "github.com/rvinnie/yookassa-sdk-go/yookassa/webhook"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockPaymentUpdater struct {
	mock.Mock
}

func (m *mockPaymentUpdater) UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error) {
	args := m.Called(ctx, yookassaID, status)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockPaymentUpdater) GrantPremium(ctx context.Context, userID int64) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

type mockPaymentFinder struct {
	mock.Mock
}

func (m *mockPaymentFinder) FindPayment(ctx context.Context, id string) (*yoopayment.Payment, error) {
	args := m.Called(ctx, id)
	if p := args.Get(0); p != nil {
		return p.(*yoopayment.Payment), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestWebhook_RejectsUntrustedIP(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	h := New(log, updater, finder)

	// An attacker connecting from public IP 203.0.113.1 trying to spoof X-Real-IP
	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBufferString("{}"))
	req.RemoteAddr = "203.0.113.1:12345"
	req.Header.Set("X-Real-IP", "185.71.76.1")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	updater.AssertNotCalled(t, "UpdatePaymentStatus")
	updater.AssertNotCalled(t, "GrantPremium")
}

func TestWebhook_AllowsValidIP_And_VerifiesPayment(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	event := yoowebhook.WebhookEvent[yoopayment.Payment]{
		Event: yoowebhook.EventPaymentSucceeded,
		Object: yoopayment.Payment{
			ID:     "pay-123",
			Status: yoopayment.Succeeded,
		},
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	finder.On("FindPayment", mock.Anything, "pay-123").Return(&yoopayment.Payment{
		ID:     "pay-123",
		Status: yoopayment.Succeeded,
	}, nil)

	updater.On("UpdatePaymentStatus", mock.Anything, "pay-123", "succeeded").Return(int64(42), nil)
	updater.On("GrantPremium", mock.Anything, int64(42)).Return(nil)

	h := New(log, updater, finder)

	// Direct request from YooKassa subnet (e.g. 185.71.76.10)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBuffer(body))
	req.RemoteAddr = "185.71.76.10:443"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	updater.AssertExpectations(t)
	finder.AssertExpectations(t)
}

func TestWebhook_RejectsWhenAPIReportsNotSucceeded(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	event := yoowebhook.WebhookEvent[yoopayment.Payment]{
		Event: yoowebhook.EventPaymentSucceeded,
		Object: yoopayment.Payment{
			ID:     "fake-payment-id",
			Status: yoopayment.Succeeded,
		},
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	// API returns payment that is actually still pending or canceled
	finder.On("FindPayment", mock.Anything, "fake-payment-id").Return(&yoopayment.Payment{
		ID:     "fake-payment-id",
		Status: yoopayment.Pending,
	}, nil)

	h := New(log, updater, finder)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBuffer(body))
	req.RemoteAddr = "127.0.0.1:443" // loopback
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	updater.AssertNotCalled(t, "UpdatePaymentStatus")
	updater.AssertNotCalled(t, "GrantPremium")
}

func TestWebhook_HandlesAPIFailureGracefully(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	event := yoowebhook.WebhookEvent[yoopayment.Payment]{
		Event: yoowebhook.EventPaymentSucceeded,
		Object: yoopayment.Payment{
			ID:     "pay-error-case",
			Status: yoopayment.Succeeded,
		},
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	finder.On("FindPayment", mock.Anything, "pay-error-case").Return(nil, errors.New("network error"))

	h := New(log, updater, finder)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBuffer(body))
	req.RemoteAddr = "127.0.0.1:443"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
	updater.AssertNotCalled(t, "UpdatePaymentStatus")
	updater.AssertNotCalled(t, "GrantPremium")
}

func TestWebhook_CanceledUpdateFails_Returns500(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	event := yoowebhook.WebhookEvent[yoopayment.Payment]{
		Event: yoowebhook.EventPaymentCanceled,
		Object: yoopayment.Payment{
			ID:     "pay-cancel-fail",
			Status: yoopayment.Canceled,
		},
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	// Запись отмены падает: обработчик обязан вернуть 5xx, чтобы шлюз повторил доставку.
	// ВАЖНО: именно int64(0) — мок делает args.Get(0).(int64), голый 0 паникует.
	updater.On("UpdatePaymentStatus", mock.Anything, "pay-cancel-fail", "canceled").
		Return(int64(0), errors.New("db is down"))

	h := New(log, updater, finder)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBuffer(body))
	req.RemoteAddr = "127.0.0.1:443" // loopback: IP-фильтр пропускает
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	updater.AssertExpectations(t)
	// Отмена не должна ничего начислять.
	updater.AssertNotCalled(t, "GrantPremium")
	// Для canceled мы не ходим в API YooKassa — платёж не мог быть успешным.
	finder.AssertNotCalled(t, "FindPayment")
}

func TestWebhook_CanceledUpdateSucceeds_Returns200(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	event := yoowebhook.WebhookEvent[yoopayment.Payment]{
		Event: yoowebhook.EventPaymentCanceled,
		Object: yoopayment.Payment{
			ID:     "pay-cancel-ok",
			Status: yoopayment.Canceled,
		},
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	updater.On("UpdatePaymentStatus", mock.Anything, "pay-cancel-ok", "canceled").
		Return(int64(7), nil)

	h := New(log, updater, finder)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBuffer(body))
	req.RemoteAddr = "127.0.0.1:443"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	updater.AssertExpectations(t)
	updater.AssertNotCalled(t, "GrantPremium")
}

// Строковые значения статусов платежа должны совпадать с константами YooKassa.
// Регрессия: в webhook-хендлере стояло "cancelled" (две буквы l) вместо "canceled",
// из-за чего отмены, пришедшие вебхуком и через sync, писались в БД по-разному.
func TestPaymentStatusConstantsMatchYooKassaSDK(t *testing.T) {
	require.Equal(t, string(yoopayment.Pending), domain.PaymentStatusPending)
	require.Equal(t, string(yoopayment.Succeeded), domain.PaymentStatusSucceeded)
	require.Equal(t, string(yoopayment.Canceled), domain.PaymentStatusCanceled)
}

// Неизвестное событие (например, refund.succeeded) не должно ломать обработку:
// подтверждаем доставку 200 и ничего не меняем.
func TestWebhook_UnknownEvent_Returns200WithoutChanges(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	updater := new(mockPaymentUpdater)
	finder := new(mockPaymentFinder)

	event := yoowebhook.WebhookEvent[yoopayment.Payment]{
		Event: yoowebhook.EventRefundSucceeded,
		Object: yoopayment.Payment{
			ID:     "pay-refund",
			Status: yoopayment.Succeeded,
		},
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	h := New(log, updater, finder)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", bytes.NewBuffer(body))
	req.RemoteAddr = "127.0.0.1:443"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	updater.AssertNotCalled(t, "UpdatePaymentStatus")
	updater.AssertNotCalled(t, "GrantPremium")
	finder.AssertNotCalled(t, "FindPayment")
}
