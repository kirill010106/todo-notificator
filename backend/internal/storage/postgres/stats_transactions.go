package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirill010106/todo-notificator/internal/domain"
)

func ensureStatsTx(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_stats (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID)
	return err
}

func applyStatsDeltaTx(ctx context.Context, tx *sql.Tx, userID int64, delta domain.StatsDelta) error {
	if err := ensureStatsTx(ctx, tx, userID); err != nil {
		return err
	}
	var points int64
	err := tx.QueryRowContext(ctx, `
		UPDATE user_stats SET
			points = GREATEST(0, COALESCE(points, 0) + $1),
			total_pomodoros = GREATEST(0, COALESCE(total_pomodoros, 0) + $2),
			total_burnt_tasks = GREATEST(0, COALESCE(total_burnt_tasks, 0) + $3),
			current_streak = CASE
				WHEN $4 THEN 0
				WHEN $5 AND (last_activity_date IS NULL OR last_activity_date < (CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$6))::date - 1) THEN 1
				WHEN $5 AND last_activity_date = (CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$6))::date - 1 THEN COALESCE(current_streak, 0) + 1
				ELSE COALESCE(current_streak, 0) END,
			best_streak = GREATEST(COALESCE(best_streak, 0), CASE
				WHEN $4 THEN 0
				WHEN $5 AND (last_activity_date IS NULL OR last_activity_date < (CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$6))::date - 1) THEN 1
				WHEN $5 AND last_activity_date = (CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$6))::date - 1 THEN COALESCE(current_streak, 0) + 1
				ELSE COALESCE(current_streak, 0) END),
			last_activity_date = CASE WHEN $5 AND NOT $4 THEN (CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$6))::date ELSE last_activity_date END,
			updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $6 RETURNING points`, delta.PointsDelta, delta.PomodorosDelta, delta.BurntTasksDelta, delta.ResetStreak, delta.IncrementStreak, userID).Scan(&points)
	if err != nil {
		return err
	}
	level, _, _, _, _, _ := domain.CalculateLevel(points)
	_, err = tx.ExecContext(ctx, `UPDATE user_stats SET level = $1 WHERE user_id = $2`, level, userID)
	return err
}

func unlockAchievementTx(ctx context.Context, tx *sql.Tx, userID int64, code string) (bool, error) {
	var definition *domain.Achievement
	for i := range domain.AvailableAchievements {
		if domain.AvailableAchievements[i].Code == code {
			definition = &domain.AvailableAchievements[i]
			break
		}
	}
	if definition == nil {
		return false, fmt.Errorf("unknown achievement: %s", code)
	}
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO achievements (user_id, code) VALUES ($1, $2) ON CONFLICT (user_id, code) DO NOTHING RETURNING id`, userID, code).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if definition.Points > 0 {
		// A bonus is not a new day of activity and must not extend the streak.
		if err := applyStatsDeltaTx(ctx, tx, userID, domain.StatsDelta{PointsDelta: definition.Points}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func checkAndUnlockAchievementsTx(ctx context.Context, tx *sql.Tx, userID int64) ([]domain.Achievement, error) {
	if err := ensureStatsTx(ctx, tx, userID); err != nil {
		return nil, err
	}
	var currentStreak, bestStreak, totalPomodoros, totalBurnt int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(current_streak, 0), COALESCE(best_streak, 0), COALESCE(total_pomodoros, 0), COALESCE(total_burnt_tasks, 0) FROM user_stats WHERE user_id = $1`, userID).
		Scan(&currentStreak, &bestStreak, &totalPomodoros, &totalBurnt); err != nil {
		return nil, err
	}
	var doneTasks, nightTasks, earlyTasks int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE EXTRACT(HOUR FROM updated_at AT TIME ZONE (SELECT timezone FROM users WHERE id=$1)) < 5), COUNT(*) FILTER (WHERE EXTRACT(HOUR FROM updated_at AT TIME ZONE (SELECT timezone FROM users WHERE id=$1)) >= 5 AND EXTRACT(HOUR FROM updated_at AT TIME ZONE (SELECT timezone FROM users WHERE id=$1)) < 8) FROM tasks WHERE user_id = $1 AND status = 'done'`, userID).
		Scan(&doneTasks, &nightTasks, &earlyTasks); err != nil {
		return nil, err
	}
	var premium bool
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(is_premium, false) FROM users WHERE id = $1`, userID).Scan(&premium); err != nil {
		return nil, err
	}
	conditions := map[string]bool{
		"first_step": doneTasks >= 1, "task_spree": doneTasks >= 10, "task_centurion": doneTasks >= 50,
		"pomodoro_rookie": totalPomodoros >= 1, "pomodoro_guru": totalPomodoros >= 20,
		"streak_three": currentStreak >= 3 || bestStreak >= 3, "streak_week": currentStreak >= 7 || bestStreak >= 7,
		"streak_month": currentStreak >= 30 || bestStreak >= 30, "phoenix": totalBurnt >= 1 && doneTasks >= 1,
		"night_owl": nightTasks >= 1, "early_bird": earlyTasks >= 1, "pro_club": premium,
	}
	var unlocked []domain.Achievement
	for _, achievement := range domain.AvailableAchievements {
		if !conditions[achievement.Code] {
			continue
		}
		added, err := unlockAchievementTx(ctx, tx, userID, achievement.Code)
		if err != nil {
			return nil, err
		}
		if added {
			unlocked = append(unlocked, achievement)
		}
	}
	return unlocked, nil
}
