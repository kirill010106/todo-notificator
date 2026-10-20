package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/clientip"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
	webhook "github.com/rvinnie/yookassa-sdk-go/yookassa/webhook"
)

var allowedYookassaCIDRs = []string{
	"185.71.76.0/27",
	"185.71.77.0/27",
	"77.75.153.0/25",
	"77.75.156.11/32",
	"77.75.156.35/32",
	"77.75.154.128/25",
	"2a02:5180::/32",
}

var allowedNets []*net.IPNet

func init() {
	for _, cidr := range allowedYookassaCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			allowedNets = append(allowedNets, ipNet)
		}
	}
}

func getClientIP(r *http.Request) string {
	return clientip.Address(r)
}

func isAllowedIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, ipNet := range allowedNets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

type PaymentUpdater interface {
	CompletePaymentAndGrantPremium(ctx context.Context, yookassaID string) (int64, error)
	UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error)
}

type PaymentFinder interface {
	FindPayment(ctx context.Context, id string) (*yoopayment.Payment, error)
}

func New(log *slog.Logger, updater PaymentUpdater, finder PaymentFinder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.payments.webhook.New"
		log := log.With(slog.String("op", op))

		clientIP := getClientIP(r)
		if clientIP != "127.0.0.1" && clientIP != "::1" && !isAllowedIP(clientIP) {
			log.Warn("rejected webhook from unauthorized IP", slog.String("ip", clientIP), slog.String("remote_addr", r.RemoteAddr))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if finder == nil {
			http.Error(w, "payment verification is temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		var event webhook.WebhookEvent[yoopayment.Payment]
		err := json.NewDecoder(r.Body).Decode(&event)
		if err != nil {
			log.Error("failed to decode webhook body", sl.Err(err))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Info("received webhook", slog.String("event", string(event.Event)), slog.String("payment_id", event.Object.ID))

		var expected yoopayment.Status
		switch event.Event {
		case webhook.EventPaymentSucceeded:
			expected = yoopayment.Succeeded
		case webhook.EventPaymentCanceled:
			expected = yoopayment.Canceled
		}
		if expected != "" {
			if event.Object.ID == "" {
				http.Error(w, "missing payment id", http.StatusBadRequest)
				return
			}
			verifiedPayment, err := finder.FindPayment(r.Context(), event.Object.ID)
			if err != nil {
				log.Error("failed to verify payment with yookassa api", sl.Err(err), slog.String("payment_id", event.Object.ID))
				http.Error(w, "payment verification failed", http.StatusBadGateway)
				return
			}
			if verifiedPayment == nil {
				log.Warn("payment verification failed: payment not found in yookassa",
					slog.String("payment_id", event.Object.ID))
				http.Error(w, "payment not found", http.StatusBadRequest)
				return
			}
			if verifiedPayment.ID != event.Object.ID || verifiedPayment.Status != expected {
				log.Warn("payment verification mismatch",
					slog.String("payment_id", event.Object.ID),
					slog.Any("actual_status", verifiedPayment.Status))
				http.Error(w, "invalid payment status", http.StatusBadRequest)
				return
			}
		}

		switch event.Event {
		case webhook.EventPaymentSucceeded:
			userID, err := updater.CompletePaymentAndGrantPremium(r.Context(), event.Object.ID)
			if err != nil {
				if errors.Is(err, storage.ErrPaymentStateConflict) {
					log.Warn("ignored conflicting terminal payment event", slog.String("payment_id", event.Object.ID))
					w.WriteHeader(http.StatusOK)
					return
				}
				log.Error("failed to complete payment and grant premium", sl.Err(err), slog.String("payment_id", event.Object.ID))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			log.Info("payment succeeded and premium granted", slog.Int64("user_id", userID), slog.String("payment_id", event.Object.ID))
		case webhook.EventPaymentCanceled:
			if _, err := updater.UpdatePaymentStatus(r.Context(), event.Object.ID, domain.PaymentStatusCanceled); err != nil {
				if errors.Is(err, storage.ErrPaymentStateConflict) {
					log.Warn("ignored conflicting terminal payment event", slog.String("payment_id", event.Object.ID))
					w.WriteHeader(http.StatusOK)
					return
				}
				log.Error("failed to update payment status to canceled",
					sl.Err(err),
					slog.String("payment_id", event.Object.ID))
				http.Error(w, "failed to update payment status", http.StatusInternalServerError)
				return
			}
		}

		// Успешная обработка (включая события, которые нас не касаются): подтверждаем доставку,
		// чтобы YooKassa не повторяла запрос. До этой точки доходят только успешные пути.
		w.WriteHeader(http.StatusOK)
	}
}
