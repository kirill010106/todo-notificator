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
	GetLatestPendingPayment(ctx context.Context, userID int64) (string, error)
	UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error)
	GrantPremium(ctx context.Context, userID int64) error
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

		paymentID, err := checker.GetLatestPendingPayment(r.Context(), userID)
		if err != nil {
			log.Error("failed to get latest pending payment", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to retrieve payment status"))
			return
		}

		if paymentID == "" {
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
				PaymentStatus: "pending",
			})
			return
		}

		payment, err := finder.FindPayment(r.Context(), paymentID)
		if err != nil {
			log.Error("failed to find payment in yookassa", sl.Err(err), slog.String("payment_id", paymentID))
			render.Status(r, http.StatusBadGateway)
			render.JSON(w, r, resp.Error("failed to check payment with payment gateway"))
			return
		}

		if payment == nil {
			render.JSON(w, r, Response{
				Response:      resp.OK(),
				IsPremium:     false,
				PaymentStatus: "not_found",
			})
			return
		}

		switch payment.Status {
		case yoopayment.Succeeded:
			_, err = checker.UpdatePaymentStatus(r.Context(), paymentID, "succeeded")
			if err != nil {
				log.Error("failed to update payment status to succeeded", sl.Err(err))
			}
			err = checker.GrantPremium(r.Context(), userID)
			if err != nil {
				log.Error("failed to grant premium", sl.Err(err))
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, resp.Error("failed to grant premium"))
				return
			}
			log.Info("payment succeeded via sync and premium granted", slog.String("payment_id", paymentID), slog.Int64("user_id", userID))

			render.JSON(w, r, Response{
				Response:      resp.OK(),
				IsPremium:     true,
				PaymentStatus: "succeeded",
			})

		case yoopayment.Canceled:
			_, _ = checker.UpdatePaymentStatus(r.Context(), paymentID, "canceled")
			render.JSON(w, r, Response{
				Response:      resp.OK(),
				IsPremium:     false,
				PaymentStatus: "canceled",
			})

		default:
			render.JSON(w, r, Response{
				Response:      resp.OK(),
				IsPremium:     false,
				PaymentStatus: string(payment.Status),
			})
		}
	}
}
