package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirill010106/todo-notificator/internal/domain"
)

func (s *Storage) CreatePayment(ctx context.Context, yookassaID string, userID int64, amount string, currency string, status string, description string) (int64, error) {
	const op = "storage.postgres.CreatePayment"

	query := `
        INSERT INTO payments (yookassa_payment_id, user_id, amount, currency, status, description)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id
    `

	var id int64
	err := s.DB.QueryRowContext(ctx, query, yookassaID, userID, amount, currency, status, description).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (s *Storage) UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error) {
	const op = "storage.postgres.UpdatePaymentStatus"

	query := `
        UPDATE payments
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE yookassa_payment_id = $2
		RETURNING user_id
    `

	var userID int64
	err := s.DB.QueryRowContext(ctx, query, status, yookassaID).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return userID, nil
}

func (s *Storage) GrantPremium(ctx context.Context, userID int64) error {
	return s.SetPremiumStatus(ctx, userID, true)
}

func (s *Storage) SetPremiumStatus(ctx context.Context, userID int64, isPremium bool) error {
	const op = "storage.postgres.SetPremiumStatus"

	stmt := `UPDATE users SET is_premium = $2 WHERE id = $1`
	_, err := s.DB.ExecContext(ctx, stmt, userID, isPremium)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) GetLatestPendingPayment(ctx context.Context, userID int64) (string, error) {
	const op = "storage.postgres.GetLatestPendingPayment"

	query := `
		SELECT yookassa_payment_id FROM payments
		WHERE user_id = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var paymentID string
	err := s.DB.QueryRowContext(ctx, query, userID, domain.PaymentStatusPending).Scan(&paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return paymentID, nil
}

