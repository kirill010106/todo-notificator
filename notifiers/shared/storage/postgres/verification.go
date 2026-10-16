package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

func (s *Storage) ClaimVerificationDelivery(ctx context.Context) (*domain.VerificationDelivery, error) {
	if _, err := s.DB.ExecContext(ctx, `UPDATE verification_email_outbox SET status='cancelled',last_error='User already verified',lease_token=NULL,lease_until=NULL WHERE status='scheduled' AND EXISTS(SELECT 1 FROM users WHERE users.id=verification_email_outbox.user_id AND users.is_verified=true) AND (lease_until IS NULL OR lease_until<CURRENT_TIMESTAMP)`); err != nil {
		return nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE verification_email_outbox SET status='failed',last_error='Verification link expired',lease_token=NULL,lease_until=NULL WHERE status='scheduled' AND expires_at<=CURRENT_TIMESTAMP AND (lease_until IS NULL OR lease_until<CURRENT_TIMESTAMP)`); err != nil {
		return nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE verification_email_outbox SET status='failed',last_error='Delivery attempt limit reached after expired lease',lease_token=NULL,lease_until=NULL WHERE status='scheduled' AND attempts>=5 AND (lease_until IS NULL OR lease_until<CURRENT_TIMESTAMP)`); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	d := &domain.VerificationDelivery{}
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return nil, err
	}
	d.LeaseToken = hex.EncodeToString(buffer)
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
 SELECT o.id FROM verification_email_outbox o JOIN users u ON u.id=o.user_id
 WHERE o.status='scheduled' AND o.attempts<5 AND u.is_verified=false AND o.expires_at>CURRENT_TIMESTAMP AND o.next_attempt_at<=CURRENT_TIMESTAMP
 AND (o.lease_until IS NULL OR o.lease_until<CURRENT_TIMESTAMP)
 ORDER BY o.next_attempt_at,o.id LIMIT 1 FOR UPDATE OF o SKIP LOCKED)
 UPDATE verification_email_outbox o
 SET attempts=attempts+1,lease_until=CURRENT_TIMESTAMP+INTERVAL '90 seconds',lease_token=$1
 FROM candidate c WHERE o.id=c.id
 RETURNING o.id,o.email,o.token,o.attempts`, d.LeaseToken).Scan(&d.ID, &d.Email, &d.Token, &d.Attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Storage) FinishVerificationDelivery(ctx context.Context, d domain.VerificationDelivery, deliveryErr error) error {
	status := "sent"
	var failure any
	var delay time.Duration
	var sentAt any = time.Now()
	if deliveryErr != nil {
		message := deliveryErr.Error()
		if len(message) > 2000 {
			message = message[:2000]
		}
		failure = message
		sentAt = nil
		status = "scheduled"
		if d.Attempt >= 5 {
			status = "failed"
		} else {
			delay = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}[d.Attempt-1]
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE verification_email_outbox
 SET status=$1,sent_at=$2,next_attempt_at=CURRENT_TIMESTAMP+($3 * INTERVAL '1 second'),last_error=$4,lease_until=NULL,lease_token=NULL
 WHERE id=$5 AND lease_token=$6 AND status='scheduled'`, status, sentAt, int64(delay.Seconds()), failure, d.ID, d.LeaseToken)
	return err
}
