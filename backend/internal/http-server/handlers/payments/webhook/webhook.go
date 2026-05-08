package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/kirill010106/todo-notificator/internal/lib/sl"
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
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ips := r.Header.Get("X-Forwarded-For"); ips != "" {
		return strings.Split(ips, ",")[0]
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
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
	UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error)
	GrantPremium(ctx context.Context, userID int64) error
}

func New(log *slog.Logger, updater PaymentUpdater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.payments.webhook.New"
		log := log.With(slog.String("op", op))

		clientIP := getClientIP(r)
		if clientIP != "127.0.0.1" && clientIP != "::1" && !isAllowedIP(clientIP) {
			log.Warn("rejected webhook from unauthorized IP", slog.String("ip", clientIP))
			http.Error(w, "forbidden", http.StatusForbidden)
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

		if event.Event == webhook.EventPaymentSucceeded {
			userID, err := updater.UpdatePaymentStatus(r.Context(), event.Object.ID, "succeeded")
			if err != nil {
				log.Error("failed to update payment status", sl.Err(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// Выдаем премиум
			err = updater.GrantPremium(r.Context(), userID)
			if err != nil {
				log.Error("failed to grant premium", sl.Err(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			log.Info("payment succeeded and premium granted", slog.Int64("user_id", userID))
		} else if event.Event == webhook.EventPaymentCanceled {
			_, _ = updater.UpdatePaymentStatus(r.Context(), event.Object.ID, "canceled")
		}

		w.WriteHeader(http.StatusOK)
	}
}
