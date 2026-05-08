package create

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"
	yooclient "github.com/rvinnie/yookassa-sdk-go/yookassa"
	yoocommon "github.com/rvinnie/yookassa-sdk-go/yookassa/common"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
)

type PaymentSaver interface {
	CreatePayment(ctx context.Context, yookassaID string, userID int64, amount string, currency string, status string, description string) (int64, error)
}

type Response struct {
	resp.Response
	ConfirmationURL string `json:"confirmation_url"`
}

func New(ctx context.Context, log *slog.Logger, paymentSaver PaymentSaver, yooClient *yooclient.Client, returnURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.payments.create.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		amount := "49.00"
		currency := "RUB"
		description := "Оплата тарифа Премиум (навсегда)"

		idempotencyKey := uuid.New().String()

		paymentHandler := yookassa.NewPaymentHandler(yooClient).WithIdempotencyKey(idempotencyKey)
		payment, err := paymentHandler.CreatePayment(ctx, &yoopayment.Payment{
			Amount: &yoocommon.Amount{
				Value:    amount,
				Currency: currency,
			},
			Capture:       true,
			PaymentMethod: yoopayment.PaymentMethodType("bank_card"),
			Description:   description,
			Confirmation: yoopayment.Redirect{
				Type:      "redirect",
				ReturnURL: returnURL,
			},
		})
		if err != nil {
			log.Error("failed to create payment in yookassa", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("Failed to create payment"))
			return
		}

		_, err = paymentSaver.CreatePayment(r.Context(), payment.ID, userID, amount, currency, string(payment.Status), description)
		if err != nil {
			log.Error("failed to save payment to db", sl.Err(err))
		}
		log.Info("payment created", slog.String("yookassa_id", payment.ID), slog.Int64("user_id", userID))

		rawURL := payment.Confirmation.(map[string]interface{})["confirmation_url"].(string)

		render.JSON(w, r, Response{
			Response:        resp.OK(),
			ConfirmationURL: rawURL,
		})
	}
}
