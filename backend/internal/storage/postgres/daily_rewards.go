package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"time"
	_ "time/tzdata"
)

func (s *Storage) GetTimezone(ctx context.Context, userID int64) (string, error) {
	var zone string
	err := s.executor(ctx).QueryRowContext(ctx, `SELECT timezone FROM users WHERE id=$1`, userID).Scan(&zone)
	return zone, err
}
func (s *Storage) SetTimezone(ctx context.Context, userID int64, zone string) error {
	if zone == "" {
		return &domain.ContractError{Code: "INVALID_TIMEZONE", Message: "Укажи часовой пояс IANA", Status: 400}
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return &domain.ContractError{Code: "INVALID_TIMEZONE", Message: "Неизвестный часовой пояс IANA", Status: 400}
	}
	_, err := s.executor(ctx).ExecContext(ctx, `UPDATE users SET timezone=$1 WHERE id=$2`, zone, userID)
	return err
}
func (s *Storage) dailyClaims(ctx context.Context, userID int64) (map[string]bool, error) {
	rows, err := s.executor(ctx).QueryContext(ctx, `SELECT code FROM daily_reward_claims WHERE user_id=$1 AND reward_date=(CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$1))::date`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	claims := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		claims[code] = true
	}
	return claims, rows.Err()
}
func (s *Storage) ClaimDailyReward(ctx context.Context, userID int64, code string) error {
	return s.withUserTx(ctx, userID, func(tx *sql.Tx) error {
		txctx := context.WithValue(ctx, transactionKey{}, userTransaction{tx, userID})
		quests, err := s.GetDailyQuests(txctx, userID)
		if err != nil {
			return err
		}
		amount := 0
		complete := false
		known := false
		if code == "daily_chest" {
			known = true
			amount = 50
			complete = true
			for _, quest := range quests {
				complete = complete && quest.Completed
			}
		}
		for _, quest := range quests {
			if quest.Code == code {
				known = true
				amount = quest.RewardXP
				complete = quest.Completed
			}
		}
		if !known {
			return &domain.ContractError{Code: "REWARD_NOT_FOUND", Message: "Награда не найдена", Status: 404}
		}
		if !complete {
			return &domain.ContractError{Code: "QUEST_NOT_COMPLETED", Message: "Сначала выполни задания", Status: 409}
		}
		var claimed string
		err = tx.QueryRowContext(ctx, `INSERT INTO daily_reward_claims(user_id,reward_date,code,xp) VALUES($1::bigint,(CURRENT_TIMESTAMP AT TIME ZONE (SELECT timezone FROM users WHERE id=$1::bigint))::date,$2,$3) ON CONFLICT DO NOTHING RETURNING code`, userID, code, amount).Scan(&claimed)
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.ContractError{Code: "REWARD_ALREADY_CLAIMED", Message: "Награда за этот день уже получена", Status: 409}
		}
		if err != nil {
			return err
		}
		if err := applyStatsDeltaTx(ctx, tx, userID, domain.StatsDelta{PointsDelta: amount}); err != nil {
			return err
		}
		if outcome := actionOutcome(ctx); outcome != nil {
			outcome.Rewards.QuestXP = amount
		}
		_, err = checkAndUnlockAchievementsTx(ctx, tx, userID)
		return err
	})
}
