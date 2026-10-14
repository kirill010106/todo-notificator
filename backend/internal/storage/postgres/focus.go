package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"math"
	"time"
)

func (s *Storage) GetFocusState(ctx context.Context, userID int64) (domain.FocusState, error) {
	var result domain.FocusState
	err := s.withReadSnapshot(ctx, userID, func(snapshot context.Context) error {
		var err error
		result, err = s.getFocusState(snapshot, userID)
		return err
	})
	return result, err
}
func (s *Storage) getFocusState(ctx context.Context, userID int64) (domain.FocusState, error) {
	db := s.executor(ctx)
	state := domain.FocusState{Phase: "idle", AvailableActions: []string{"start"}}
	if err := db.QueryRowContext(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&state.ServerNow); err != nil {
		return state, err
	}
	var recovery *time.Time
	err := db.QueryRowContext(ctx, `SELECT completed_in_cycle,recovery_ends_at FROM focus_states WHERE user_id=$1`, userID).Scan(&state.CompletedInCycle, &recovery)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	session, err := s.GetActivePomodoroSession(ctx, userID)
	if err != nil && !errors.Is(err, storage.ErrSessionNotFound) {
		return state, err
	}
	if session == nil {
		if recovery != nil {
			state.Phase = "recovery_break"
			state.PhaseEndsAt = recovery
			state.AvailableActions = []string{"resume", "finish_rest"}
		}
		return state, nil
	}
	state.Session = session
	state.Phase = "focus"
	if session.StartedAt == nil {
		return state, errors.New("active session has no start timestamp")
	}
	end := session.StartedAt.Add(time.Duration(session.DurationMinutes) * time.Minute)
	state.PhaseEndsAt = &end
	state.RemainingWorkSeconds = int(math.Max(0, math.Min(float64(session.DurationMinutes*60), math.Ceil(end.Sub(state.ServerNow).Seconds()))))
	state.AvailableActions = []string{"complete_session", "abandon"}
	if session.TaskID != nil {
		state.AvailableActions = append(state.AvailableActions, "complete_task")
	}
	if session.BreaksUsed == 0 {
		state.AvailableActions = append(state.AvailableActions, "take_break")
	}
	var breakEnd *time.Time
	if err := db.QueryRowContext(ctx, `SELECT break_ends_at FROM pomodoro_sessions WHERE id=$1 AND user_id=$2`, session.ID, userID).Scan(&breakEnd); err != nil {
		return state, err
	}
	if breakEnd != nil && breakEnd.After(state.ServerNow) {
		state.Phase = "extra_break"
		state.PhaseEndsAt = breakEnd
		state.AvailableActions = []string{"resume"}
	}
	return state, nil
}

func (s *Storage) ResumeFocus(ctx context.Context, userID int64) error {
	return s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		txctx := context.WithValue(ctx, transactionKey{}, userTransaction{tx, userID})
		focus, err := s.GetFocusState(txctx, userID)
		if err != nil {
			return err
		}
		if focus.Phase == "extra_break" {
			_, err = tx.ExecContext(ctx, `UPDATE pomodoro_sessions SET break_ends_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`, focus.Session.ID, userID)
			return err
		}
		if focus.Phase != "recovery_break" {
			return &domain.ContractError{Code: "FOCUS_NOT_ON_BREAK", Message: "Нет перерыва для продолжения", Status: 409}
		}
		var taskID *int64
		if err := tx.QueryRowContext(ctx, `SELECT recovery_task_id FROM focus_states WHERE user_id=$1`, userID).Scan(&taskID); err != nil {
			return err
		}
		// Completed or burnt tasks are not selected for the next focus session.
		if taskID != nil {
			var status string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id=$1 AND user_id=$2`, *taskID, userID).Scan(&status); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if status != domain.TaskStatusPending {
				taskID = nil
			}
		}
		_, err = s.StartPomodoroSession(txctx, userID, taskID)
		return err
	})
}
func (s *Storage) FinishFocusRest(ctx context.Context, userID int64) error {
	return s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE focus_states SET recovery_ends_at=NULL,recovery_task_id=NULL WHERE user_id=$1`, userID)
		return err
	})
}
func (s *Storage) updateFocusAfterFinish(ctx context.Context, tx *sql.Tx, userID int64, taskID *int64, action string, eligible bool) error {
	if action != domain.PomodoroStatusCompleted || !eligible {
		_, err := tx.ExecContext(ctx, `UPDATE focus_states SET recovery_ends_at=NULL,recovery_task_id=NULL WHERE user_id=$1`, userID)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO focus_states(user_id,completed_in_cycle,recovery_ends_at,recovery_task_id)
        VALUES($1,1,CURRENT_TIMESTAMP+INTERVAL '5 minutes',$2)
        ON CONFLICT(user_id) DO UPDATE SET
        completed_in_cycle=(focus_states.completed_in_cycle+1)%4,
        recovery_ends_at=CURRENT_TIMESTAMP+CASE WHEN focus_states.completed_in_cycle=3 THEN INTERVAL '15 minutes' ELSE INTERVAL '5 minutes' END,
        recovery_task_id=$2`, userID, taskID)
	return err
}
