package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
	"time"
)

func (s *Storage) ClaimReminder(ctx context.Context) (*domain.ReminderDelivery, error) {
	// Retire exhausted leases in a separate short transaction. Do not carry
	// reminder locks into the task-first claim transaction below.
	if _, err := s.DB.ExecContext(ctx, `UPDATE task_reminders SET status='failed',last_error='Worker interrupted during final attempt',lease_token=NULL,lease_until=NULL WHERE status='scheduled' AND attempts>=5 AND lease_until<CURRENT_TIMESTAMP`); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	d := domain.ReminderDelivery{}
	// Lock the task first, consistently with task edits/cancellation, then its reminder.
	var taskID int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM tasks t JOIN users u ON u.id=t.user_id
 WHERE t.status='pending' AND u.is_verified=true AND EXISTS(SELECT 1 FROM task_reminders r WHERE r.task_id=t.id AND r.status='scheduled' AND r.attempts<5 AND r.next_attempt_at<=CURRENT_TIMESTAMP AND (r.lease_until IS NULL OR r.lease_until<CURRENT_TIMESTAMP))
 ORDER BY t.id LIMIT 1 FOR UPDATE OF t SKIP LOCKED`).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return nil, err
	}
	d.LeaseToken = hex.EncodeToString(token)
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
 SELECT id FROM task_reminders WHERE task_id=$1 AND status='scheduled' AND attempts<5
 AND next_attempt_at<=CURRENT_TIMESTAMP AND (lease_until IS NULL OR lease_until<CURRENT_TIMESTAMP)
 ORDER BY next_attempt_at,id LIMIT 1 FOR UPDATE SKIP LOCKED)
 UPDATE task_reminders r SET lease_until=CURRENT_TIMESTAMP+INTERVAL '90 seconds',lease_token=$2,attempts=attempts+1
 FROM candidate c WHERE r.id=c.id RETURNING r.id,r.scheduled_at,r.offset_minutes,r.attempts`, taskID, d.LeaseToken).
		Scan(&d.ID, &d.ScheduledAt, &d.OffsetMinutes, &d.Attempt)
	// A concurrent claim may have committed after the task-selection snapshot.
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.user_id,t.title,t.description,t.deadline,t.status,u.id,u.email,u.timezone FROM tasks t JOIN users u ON u.id=t.user_id WHERE t.id=$1`, taskID).
		Scan(&d.Task.ID, &d.Task.UserID, &d.Task.Title, &d.Task.Description, &d.Task.Deadline, &d.Task.Status, &d.User.ID, &d.User.Email, &d.User.Timezone)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO reminder_delivery_attempts(reminder_id,attempt_number,lease_token,error,succeeded) VALUES($1,$2,$3,'Worker interrupted before acknowledgement',false)`, d.ID, d.Attempt, d.LeaseToken); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Storage) ReminderReady(ctx context.Context, d domain.ReminderDelivery) (bool, error) {
	var ready bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM task_reminders r JOIN tasks t ON t.id=r.task_id JOIN users u ON u.id=t.user_id WHERE r.id=$1 AND r.lease_token=$2 AND r.status='scheduled' AND r.lease_until>CURRENT_TIMESTAMP AND t.status='pending' AND u.is_verified=true)`, d.ID, d.LeaseToken).Scan(&ready)
	return ready, err
}

func (s *Storage) FinishReminder(ctx context.Context, d domain.ReminderDelivery, sendErr error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Match task-edit lock order; cancellation invalidates the fencing token.
	var taskID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id=$1 FOR UPDATE`, d.Task.ID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var success = sendErr == nil
	var failure any
	var status = "sent"
	var delay int64
	if sendErr != nil {
		message := sendErr.Error()
		if len(message) > 2000 {
			message = message[:2000]
		}
		failure = message
		status = "scheduled"
		delays := []int64{60, 300, 900, 3600}
		if d.Attempt >= 5 {
			status = "failed"
		} else {
			delay = delays[d.Attempt-1]
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE task_reminders SET status=$1,sent_at=CASE WHEN $2 THEN CURRENT_TIMESTAMP ELSE sent_at END,
 next_attempt_at=CURRENT_TIMESTAMP+($3 * INTERVAL '1 second'),last_error=$4,lease_until=NULL,lease_token=NULL
 WHERE id=$5 AND lease_token=$6 AND status='scheduled' AND lease_until>CURRENT_TIMESTAMP`, status, success, delay, failure, d.ID, d.LeaseToken)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE reminder_delivery_attempts SET succeeded=$1,error=$2 WHERE reminder_id=$3 AND attempt_number=$4 AND lease_token=$5`, success, failure, d.ID, d.Attempt, d.LeaseToken); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Storage) NextReminderAt(ctx context.Context) (*time.Time, error) {
	var next *time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT MIN(GREATEST(r.next_attempt_at,COALESCE(r.lease_until,r.next_attempt_at))) FROM task_reminders r JOIN tasks t ON t.id=r.task_id JOIN users u ON u.id=t.user_id WHERE r.status='scheduled' AND t.status='pending' AND u.is_verified=true`).Scan(&next)
	return next, err
}
