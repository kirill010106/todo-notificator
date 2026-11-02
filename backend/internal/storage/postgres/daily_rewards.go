package postgres

import (
	"context"
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
	if zone == "" || zone == "Local" {
		return &domain.ContractError{Code: "INVALID_TIMEZONE", Message: "Укажи часовой пояс IANA", Status: 400}
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return &domain.ContractError{Code: "INVALID_TIMEZONE", Message: "Неизвестный часовой пояс IANA", Status: 400}
	}
	_, err := s.executor(ctx).ExecContext(ctx, `UPDATE users SET timezone=$1 WHERE id=$2`, zone, userID)
	return err
}

// Retained for old clients; historical claims are never modified.
func (s *Storage) ClaimDailyReward(ctx context.Context, userID int64, code string) error {
	return &domain.ContractError{Code: "MECHANIC_RETIRED", Message: "Ежедневные награды больше не используются", Status: 410}
}
