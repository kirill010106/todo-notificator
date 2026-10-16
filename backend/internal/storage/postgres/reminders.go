package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"time"
)

// One correlated aggregate avoids N+1 queries and participates in Today's snapshot.
const reminderJSON = `COALESCE((SELECT json_agg(json_build_object('id',r.id,'offset_minutes',r.offset_minutes,'scheduled_at',r.scheduled_at,'status',r.status,'sent_at',r.sent_at,'legacy_absolute',r.legacy_absolute) ORDER BY r.scheduled_at,r.id) FROM task_reminders r WHERE r.task_id=tasks.id AND r.status<>'cancelled'),'[]')`

func decodeReminders(raw []byte, task *domain.Task) error {
	task.Reminders = []domain.TaskReminder{}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &task.Reminders)
}

func (s *Storage) SaveTask(ctx context.Context, task domain.Task) (int64, error) {
	var id int64
	err := s.withUserTx(ctx, task.UserID, func(tx *sql.Tx) error {
		txctx := context.WithValue(ctx, transactionKey{}, userTransaction{tx, task.UserID})
		var err error
		id, err = s.saveTaskRow(txctx, task)
		if err != nil {
			return err
		}
		inputs := task.ReminderInputs
		return s.replaceTaskReminders(txctx, tx, task.UserID, id, domain.TaskUpdate{Reminders: &inputs})
	})
	return id, err
}

func (s *Storage) replaceTaskReminders(ctx context.Context, tx *sql.Tx, userID, taskID int64, update domain.TaskUpdate) error {
	if update.Reminders == nil && update.Deadline == nil && !update.ClearDeadline {
		return nil
	}
	var deadline *time.Time
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT deadline,status FROM tasks WHERE user_id=$1 AND id=$2 FOR UPDATE`, userID, taskID).Scan(&deadline, &status); err != nil {
		return err
	}
	var premium bool
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT is_premium,clock_timestamp() FROM users WHERE id=$1`, userID).Scan(&premium, &now); err != nil {
		return err
	}
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT `+reminderJSON+` FROM tasks WHERE id=$1`, taskID).Scan(&raw); err != nil {
		return err
	}
	task := domain.Task{}
	if err := decodeReminders(raw, &task); err != nil {
		return err
	}
	changed := update.Deadline != nil || update.ClearDeadline
	if update.ClearDeadline {
		deadline = nil
	} else if update.Deadline != nil {
		deadline = update.Deadline
	}
	desired, err := domain.PlanReminders(task.Reminders, update.Reminders, deadline, changed, premium, now)
	if err != nil {
		return err
	}
	if update.Status != nil {
		status = *update.Status
	}
	keep := map[int64]bool{}
	for _, r := range desired {
		if r.ID == 0 && status != "pending" {
			return domain.ReminderError("reminders", "Новые напоминания можно добавить только к задаче в работе")
		}
		if r.ID == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO task_reminders(task_id,offset_minutes,scheduled_at,next_attempt_at) VALUES($1,$2,$3,$3)`, taskID, r.OffsetMinutes, r.ScheduledAt); err != nil {
				return err
			}
		} else {
			keep[r.ID] = true
			// Do not disturb a live lease or retry when saving an unchanged form.
			if _, err := tx.ExecContext(ctx, `UPDATE task_reminders SET offset_minutes=$1,legacy_absolute=$2,scheduled_at=$3,status=$4,
    attempts=0,next_attempt_at=$3,lease_token=NULL,lease_until=NULL,last_error=NULL
    WHERE id=$5 AND task_id=$6 AND (offset_minutes IS DISTINCT FROM $1 OR legacy_absolute IS DISTINCT FROM $2 OR scheduled_at IS DISTINCT FROM $3)`, r.OffsetMinutes, r.LegacyAbsolute, r.ScheduledAt, r.Status, r.ID, taskID); err != nil {
				return err
			}
		}
	}
	for _, old := range task.Reminders {
		if !keep[old.ID] {
			if _, err := tx.ExecContext(ctx, `UPDATE task_reminders SET status='cancelled',lease_token=NULL,lease_until=NULL WHERE id=$1 AND task_id=$2`, old.ID, taskID); err != nil {
				return err
			}
		}
	}
	return nil
}
