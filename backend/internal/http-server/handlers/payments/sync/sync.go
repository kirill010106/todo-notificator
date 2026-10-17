package sync

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
)

type PaymentStatusChecker interface {
	GetPendingPayments(ctx context.Context, userID int64) ([]string, error)
	CompletePaymentAndGrantPremium(ctx context.Context, yookassaID string) (int64, error)
	UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
}

type PaymentFinder interface {
	FindPayment(ctx context.Context, id string) (*yoopayment.Payment, error)
}

type Response struct {
	resp.Response
	IsPremium     bool   `json:"is_premium"`
	PaymentStatus string `json:"payment_status,omitempty"`
}

func New(log *slog.Logger, checker PaymentStatusChecker, finder PaymentFinder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.payments.sync.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		user, err := checker.GetUserByID(r.Context(), userID)
		if err == nil && user != nil && user.IsPremium {
			render.JSON(w, r, Response{
				Response:  resp.OK(),
				IsPremium: true,
			})
			return
		}

		paymentIDs, err := checker.GetPendingPayments(r.Context(), userID)
		if err != nil {
			log.Error("failed to get pending payments", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to retrieve payment status"))
			return
		}

		if len(paymentIDs) == 0 {
			isPrem := false
			if user != nil {
				isPrem = user.IsPremium
			}
			render.JSON(w, r, Response{
				Response:  resp.OK(),
				IsPremium: isPrem,
			})
			return
		}

		if finder == nil {
			log.Warn("yookassa client is not configured for payment verification")
			render.JSON(w, r, Response{
				Response:      resp.OK(),
				IsPremium:     false,
				PaymentStatus: domain.PaymentStatusPending,
			})
			return
		}

		latestStatus := domain.PaymentStatusPending
		for index, paymentID := range paymentIDs {
			payment, err := finder.FindPayment(r.Context(), paymentID)
			if err != nil {
				log.Error("failed to find payment in yookassa", sl.Err(err), slog.String("payment_id", paymentID))
				render.Status(r, http.StatusBadGateway)
				render.JSON(w, r, resp.Error("failed to check payment with payment gateway"))
				return
			}
			if payment == nil {
				if index == 0 {
					latestStatus = "not_found"
				}
				continue
			}
			if index == 0 {
				latestStatus = string(payment.Status)
			}
			switch payment.Status {
			case yoopayment.Succeeded:
				grantedUserID, err := checker.CompletePaymentAndGrantPremium(r.Context(), paymentID)
				if err != nil {
					log.Error("failed to complete payment and grant premium via sync", sl.Err(err))
					render.Status(r, http.StatusInternalServerError)
					render.JSON(w, r, resp.Error("failed to complete payment"))
					return
				}
				log.Info("payment succeeded via sync and premium granted", slog.String("payment_id", paymentID), slog.Int64("user_id", grantedUserID))
				render.JSON(w, r, Response{Response: resp.OK(), IsPremium: true, PaymentStatus: domain.PaymentStatusSucceeded})
				return
			case yoopayment.Canceled:
				if _, err := checker.UpdatePaymentStatus(r.Context(), paymentID, domain.PaymentStatusCanceled); err != nil {
					log.Error("failed to update payment status to canceled", sl.Err(err), slog.String("payment_id", paymentID))
					render.Status(r, http.StatusInternalServerError)
					render.JSON(w, r, resp.Error("failed to update payment status"))
					return
				}
			}
		}
		render.JSON(w, r, Response{Response: resp.OK(), IsPremium: false, PaymentStatus: latestStatus})
	}
}
