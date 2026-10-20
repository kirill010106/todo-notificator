package webhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/kirill010106/todo-notificator/internal/storage"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCanceledWebhookRequiresMatchingProviderPayment(t *testing.T) {
	for _, test := range []struct {
		name    string
		payment *yoopayment.Payment
		err     error
		code    int
	}{
		{"pending", &yoopayment.Payment{ID: "payment", Status: yoopayment.Pending}, nil, 400},
		{"succeeded", &yoopayment.Payment{ID: "payment", Status: yoopayment.Succeeded}, nil, 400},
		{"different id", &yoopayment.Payment{ID: "another", Status: yoopayment.Canceled}, nil, 400},
		{"not found", nil, nil, 400}, {"provider unavailable", nil, errors.New("provider down"), 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			updater, finder := new(mockPaymentUpdater), new(mockPaymentFinder)
			finder.On("FindPayment", mock.Anything, "payment").Return(test.payment, test.err).Once()
			body := []byte(`{"event":"payment.canceled","object":{"id":"payment","status":"canceled"}}`)
			r := httptest.NewRequest("POST", "/webhooks/yookassa", bytes.NewReader(body))
			r.RemoteAddr = "185.71.76.10:443"
			w := httptest.NewRecorder()
			New(slog.New(slog.DiscardHandler), updater, finder)(w, r)
			require.Equal(t, test.code, w.Code)
			finder.AssertExpectations(t)
			updater.AssertNotCalled(t, "UpdatePaymentStatus")
			updater.AssertNotCalled(t, "CompletePaymentAndGrantPremium")
		})
	}
}

func TestWebhookAcknowledgesConflictingTerminalEventWithoutReversal(t *testing.T) {
	for _, status := range []yoopayment.Status{yoopayment.Succeeded, yoopayment.Canceled} {
		updater, finder := new(mockPaymentUpdater), new(mockPaymentFinder)
		finder.On("FindPayment", mock.Anything, "payment").Return(&yoopayment.Payment{ID: "payment", Status: status}, nil).Once()
		if status == yoopayment.Succeeded {
			updater.On("CompletePaymentAndGrantPremium", mock.Anything, "payment").Return(int64(0), storage.ErrPaymentStateConflict).Once()
		} else {
			updater.On("UpdatePaymentStatus", mock.Anything, "payment", "canceled").Return(int64(0), storage.ErrPaymentStateConflict).Once()
		}
		body, err := json.Marshal(map[string]any{"event": "payment." + string(status), "object": map[string]string{"id": "payment"}})
		require.NoError(t, err)
		r := httptest.NewRequest("POST", "/webhooks/yookassa", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:443"
		w := httptest.NewRecorder()
		New(slog.New(slog.DiscardHandler), updater, finder)(w, r)
		require.Equal(t, 200, w.Code)
		finder.AssertExpectations(t)
		updater.AssertExpectations(t)
	}
}

func TestWebhookRejectsWrongPaymentIDForEitherTerminalStatus(t *testing.T) {
	for _, status := range []yoopayment.Status{yoopayment.Succeeded, yoopayment.Canceled} {
		finder := new(mockPaymentFinder)
		finder.On("FindPayment", mock.Anything, "expected").Return(&yoopayment.Payment{ID: "another", Status: status}, nil).Once()
		body, err := json.Marshal(map[string]any{"event": "payment." + string(status), "object": map[string]string{"id": "expected"}})
		require.NoError(t, err)
		r := httptest.NewRequest("POST", "/webhooks/yookassa", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:443"
		w := httptest.NewRecorder()
		// nil updater makes any attempted state change fail immediately.
		New(slog.New(slog.DiscardHandler), nil, finder)(w, r)
		require.Equal(t, 400, w.Code)
		finder.AssertExpectations(t)
	}
}
