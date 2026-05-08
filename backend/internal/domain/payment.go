package domain

import "time"

const (
	PaymentStatusPending   = "pending"
	PaymentStatusSucceeded = "succeeded"
	PaymentStatusCanceled  = "canceled"
)

type Payment struct {
	D                 int64     `json:"id"`
	YooKassaPaymentID string    `json:"yookassa_payment_id"`
	UserID            int64     `json:"user_id"`
	Amount            string    `json:"amount"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	Description       string    `json:"description"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
