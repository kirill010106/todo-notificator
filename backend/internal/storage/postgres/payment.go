package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

func (s *Storage) CreatePayment(ctx context.Context, yookassaID string, userID int64, amount string, currency string, status string, description string) (int64, error) {
	const op = "storage.postgres.CreatePayment"

	query := `
		INSERT INTO payments (yookassa_payment_id, user_id, amount, currency, status, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (yookassa_payment_id) DO UPDATE SET status=CASE
			WHEN payments.status IN ('succeeded','canceled') THEN payments.status ELSE EXCLUDED.status END
		WHERE payments.user_id=EXCLUDED.user_id AND payments.amount=EXCLUDED.amount AND payments.currency=EXCLUDED.currency
		RETURNING id
	`

	var id int64
	err := s.executor(ctx).QueryRowContext(ctx, query, yookassaID, userID, amount, currency, status, description).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (s *Storage) UpdatePaymentStatus(ctx context.Context, yookassaID string, status string) (int64, error) {
	const op = "storage.postgres.UpdatePaymentStatus"
	// Success must always go through the atomic Premium-granting operation.
	if status != domain.PaymentStatusCanceled {
		return 0, fmt.Errorf("%s: %w", op, storage.ErrPaymentStateConflict)
	}

	query := `
        UPDATE payments
		SET status = $1::varchar, updated_at = CASE WHEN status=$1::varchar THEN updated_at ELSE CURRENT_TIMESTAMP END
		WHERE yookassa_payment_id = $2 AND status IN ('pending','waiting_for_capture','canceled')
		RETURNING user_id
    `

	var userID int64
	err := s.DB.QueryRowContext(ctx, query, status, yookassaID).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, paymentTransitionError(ctx, s.DB, yookassaID, status, err))
	}

	return userID, nil
}

func paymentTransitionError(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, paymentID, target string, err error) error {
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var current string
	err = query.QueryRowContext(ctx, `SELECT status FROM payments WHERE yookassa_payment_id=$1`, paymentID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && current != target && (current == domain.PaymentStatusSucceeded || current == domain.PaymentStatusCanceled) {
		return storage.ErrPaymentStateConflict
	}
	// A row created after the conditional UPDATE still needs processing. Treat
	// it as retryable, rather than acknowledging an event without applying it.
	return storage.ErrPaymentNotFound
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

func (s *Storage) GetPendingPayments(ctx context.Context, userID int64) ([]string, error) {
	const op = "storage.postgres.GetPendingPayments"
	rows, err := s.DB.QueryContext(ctx, `SELECT yookassa_payment_id FROM payments WHERE user_id=$1 AND status=$2 ORDER BY created_at DESC,id DESC`, userID, domain.PaymentStatusPending)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: rows: %w", op, err)
	}
	return ids, nil
}

func (s *Storage) CompletePaymentAndGrantPremium(ctx context.Context, yookassaID string) (int64, error) {
	const op = "storage.postgres.CompletePaymentAndGrantPremium"

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// 1. Обновляем статус платежа и получаем user_id
	queryPayment := `
		UPDATE payments
		SET status = $1::varchar, updated_at = CASE WHEN status=$1::varchar THEN updated_at ELSE CURRENT_TIMESTAMP END
		WHERE yookassa_payment_id = $2 AND status IN ('pending','waiting_for_capture','succeeded')
		RETURNING user_id
	`
	var userID int64
	err = tx.QueryRowContext(ctx, queryPayment, domain.PaymentStatusSucceeded, yookassaID).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("%s: update payment status: %w", op, paymentTransitionError(ctx, tx, yookassaID, domain.PaymentStatusSucceeded, err))
	}

	// 2. Выдаем премиум пользователю
	stmtUser := `UPDATE users SET is_premium = true WHERE id = $1`
	_, err = tx.ExecContext(ctx, stmtUser, userID)
	if err != nil {
		return 0, fmt.Errorf("%s: grant premium to user %d: %w", op, userID, err)
	}

	// 3. Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s: commit tx: %w", op, err)
	}

	return userID, nil
}
