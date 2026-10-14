package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
)

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Serialize business actions for each user. All state and reward changes share
// the transaction; failures roll back both, including the reward claim.
func (s *Storage) withUserTx(ctx context.Context, userID int64, action func(*sql.Tx) error) error {
	if existing, ok := ctx.Value(transactionKey{}).(userTransaction); ok {
		if existing.userID != userID {
			return errors.New("transaction user mismatch")
		}
		return action(existing.tx)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.ErrUserNotFound
		}
		return err
	}
	if err := action(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func lockTask(ctx context.Context, tx *sql.Tx, userID, taskID int64) (domain.Task, error) {
	var task domain.Task
	err := tx.QueryRowContext(ctx, `SELECT id, status, reward_claimed FROM tasks WHERE user_id = $1 AND id = $2 FOR UPDATE`, userID, taskID).
		Scan(&task.ID, &task.Status, &task.RewardClaimed)
	if errors.Is(err, sql.ErrNoRows) {
		return task, storage.ErrTaskNotFound
	}
	return task, err
}

func (s *Storage) updateTaskWithRewards(ctx context.Context, userID, taskID int64, update domain.TaskUpdate) error {
	return s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		task, err := lockTask(ctx, tx, userID, taskID)
		if err != nil {
			return err
		}
		delta := domain.StatsDelta{}
		// RewardClaimed is determined from locked server state, never a request.
		update.RewardClaimed = false
		if update.Status != nil && *update.Status == domain.TaskStatusDone && task.Status != domain.TaskStatusDone && !task.RewardClaimed {
			update.RewardClaimed = true
			delta.PointsDelta = domain.TaskCompleteReward
			delta.IncrementStreak = true
		}
		if err := updateTaskSQL(ctx, tx, userID, taskID, update); err != nil {
			return err
		}
		if delta.PointsDelta != 0 {
			if err := applyStatsDeltaTx(ctx, tx, userID, delta); err != nil {
				return err
			}
		}
		if outcome := actionOutcome(ctx); outcome != nil {
			outcome.Rewards.TaskXP += delta.PointsDelta
			outcome.Result.TaskCompleted = update.Status != nil && *update.Status == domain.TaskStatusDone
			outcome.Result.ChangedTaskIDs = append(outcome.Result.ChangedTaskIDs, taskID)
		}
		_, err = checkAndUnlockAchievementsTx(ctx, tx, userID)
		return err
	})
}

func (s *Storage) bulkCompleteWithRewards(ctx context.Context, userID int64, taskIDs []int64) (int64, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}
	var affected int64
	err := s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		placeholders := make([]string, len(taskIDs))
		args := []any{userID}
		for i, id := range taskIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		query := fmt.Sprintf(`
		WITH candidates AS (
			SELECT id, reward_claimed FROM tasks
			WHERE user_id = $1 AND id IN (%s) AND status <> 'done'
			ORDER BY id FOR UPDATE
		), updated AS (
			UPDATE tasks t SET status = 'done', reminder_at = NULL,
				reward_claimed = true, updated_at = CURRENT_TIMESTAMP
			FROM candidates c WHERE t.id = c.id
			RETURNING t.id, NOT c.reward_claimed AS rewarded
		)
		SELECT COUNT(*), COUNT(*) FILTER (WHERE rewarded) FROM updated`, strings.Join(placeholders, ", "))
		var rewarded int
		if outcome := actionOutcome(ctx); outcome != nil {
			query = strings.Replace(query, "COUNT(*) FILTER (WHERE rewarded) FROM updated", "COUNT(*) FILTER (WHERE rewarded), COALESCE(json_agg(id),'[]') FROM updated", 1)
			var ids []byte
			if err := tx.QueryRowContext(ctx, query, args...).Scan(&affected, &rewarded, &ids); err != nil {
				return err
			}
			if err := json.Unmarshal(ids, &outcome.Result.ChangedTaskIDs); err != nil {
				return err
			}
			changed := map[int64]bool{}
			for _, id := range outcome.Result.ChangedTaskIDs {
				changed[id] = true
			}
			seen := map[int64]bool{}
			for _, id := range taskIDs {
				if !seen[id] && !changed[id] {
					outcome.Result.SkippedTaskIDs = append(outcome.Result.SkippedTaskIDs, id)
				}
				seen[id] = true
			}
			outcome.Rewards.TaskXP = rewarded * domain.TaskCompleteReward
			outcome.Result.TaskCompleted = affected > 0
		} else if err := tx.QueryRowContext(ctx, query, args...).Scan(&affected, &rewarded); err != nil {
			return err
		}
		if rewarded > 0 {
			if err := applyStatsDeltaTx(ctx, tx, userID, domain.StatsDelta{PointsDelta: rewarded * domain.TaskCompleteReward, IncrementStreak: true}); err != nil {
				return err
			}
		}
		_, err := checkAndUnlockAchievementsTx(ctx, tx, userID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// FinishPomodoroSession checks the exact requested session and awards each
// completed session once. Lock tasks before sessions, matching the deletion
// trigger's order and avoiding a deadlock with concurrent task deletion.
func (s *Storage) FinishPomodoroSession(ctx context.Context, userID, sessionID int64, action string, finishTask bool) error {
	if action != domain.PomodoroStatusCompleted && action != domain.PomodoroStatusAbandoned {
		return fmt.Errorf("invalid pomodoro action: %s", action)
	}
	return s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		var taskID *int64
		if err := tx.QueryRowContext(ctx, `SELECT task_id FROM pomodoro_sessions WHERE id = $1 AND user_id = $2`, sessionID, userID).Scan(&taskID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return storage.ErrSessionNotFound
			}
			return err
		}
		var task domain.Task
		if taskID != nil {
			var err error
			task, err = lockTask(ctx, tx, userID, *taskID)
			if err != nil && !errors.Is(err, storage.ErrTaskNotFound) {
				return err
			}
		}
		var status string
		var eligible bool
		if err := tx.QueryRowContext(ctx, `SELECT task_id, status, COALESCE(started_at <= CURRENT_TIMESTAMP - INTERVAL '24 minutes', false) FROM pomodoro_sessions WHERE id = $1 AND user_id = $2 FOR UPDATE`, sessionID, userID).
			Scan(&taskID, &status, &eligible); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return storage.ErrSessionNotFound
			}
			return err
		}
		if status != domain.PomodoroStatusActive {
			return storage.ErrSessionNotFound
		}
		if taskID != nil && task.ID != *taskID {
			return storage.ErrTaskNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE pomodoro_sessions SET status = $1, completed_at = CURRENT_TIMESTAMP WHERE id = $2 AND user_id = $3`, action, sessionID, userID); err != nil {
			return err
		}
		delta := domain.StatsDelta{}
		if action == domain.PomodoroStatusCompleted && eligible {
			delta.PointsDelta = domain.PomodoroRewardPoints
			delta.PomodorosDelta = 1
			delta.IncrementStreak = true
		} else if action == domain.PomodoroStatusAbandoned && taskID != nil {
			delta.PointsDelta = domain.PomodoroPenaltyPoints
		}
		if taskID != nil {
			update := domain.TaskUpdate{IncrementPomodorosTaken: action == domain.PomodoroStatusCompleted && eligible}
			if finishTask {
				newStatus := domain.TaskStatusDone
				if action == domain.PomodoroStatusAbandoned {
					newStatus = domain.TaskStatusBurnt
					if task.Status != domain.TaskStatusBurnt {
						delta.BurntTasksDelta = 1
						delta.ResetStreak = true
					}
				} else if task.Status != domain.TaskStatusDone && !task.RewardClaimed {
					update.RewardClaimed = true
					delta.PointsDelta += domain.TaskCompleteReward
					delta.IncrementStreak = true
				}
				update.Status = &newStatus
			}
			if err := updateTaskSQL(ctx, tx, userID, *taskID, update); err != nil {
				return err
			}
		}
		if delta != (domain.StatsDelta{}) {
			if err := applyStatsDeltaTx(ctx, tx, userID, delta); err != nil {
				return err
			}
		}
		if err := s.updateFocusAfterFinish(ctx, tx, userID, taskID, action, eligible); err != nil {
			return err
		}
		if outcome := actionOutcome(ctx); outcome != nil {
			outcome.Result.PomodoroCounted = action == domain.PomodoroStatusCompleted && eligible
			outcome.Result.TaskCompleted = finishTask && taskID != nil && action == domain.PomodoroStatusCompleted
			if taskID != nil && finishTask {
				outcome.Result.ChangedTaskIDs = append(outcome.Result.ChangedTaskIDs, *taskID)
			}
			if action == domain.PomodoroStatusCompleted {
				if eligible {
					outcome.Rewards.PomodoroXP = domain.PomodoroRewardPoints
				}
				outcome.Rewards.TaskXP = delta.PointsDelta - outcome.Rewards.PomodoroXP
			} else {
				outcome.Rewards.PenaltyXP = -delta.PointsDelta
			}
		}
		_, err := checkAndUnlockAchievementsTx(ctx, tx, userID)
		return err
	})
}
