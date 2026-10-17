package postgres

import (
	"context"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"time"
)

func (s *Storage) GetToday(ctx context.Context, userID int64, zone string, limit, todayOffset, overdueOffset int) (domain.TodayState, error) {
	var result domain.TodayState
	err := s.withReadSnapshot(ctx, userID, func(snapshot context.Context) error {
		var err error
		result, err = s.getToday(snapshot, userID, zone, limit, todayOffset, overdueOffset)
		return err
	})
	return result, err
}
func (s *Storage) getToday(ctx context.Context, userID int64, zone string, limit, todayOffset, overdueOffset int) (domain.TodayState, error) {
	var result domain.TodayState
	if zone == "" {
		var err error
		zone, err = s.GetTimezone(ctx, userID)
		if err != nil {
			return result, err
		}
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return result, &domain.ContractError{Code: "INVALID_TIMEZONE", Message: "Неизвестный часовой пояс IANA", Status: 400}
	}
	if err := s.executor(ctx).QueryRowContext(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&result.ServerNow); err != nil {
		return result, err
	}
	result.Timezone = zone
	local := result.ServerNow.In(location)
	end := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location)
	pending, sort, order := "pending", "deadline", "asc"
	todayFilter := domain.TaskFilter{Status: &pending, SortBy: &sort, Order: &order, DeadlineFrom: &result.ServerNow, DeadlineBefore: &end}
	tasks, total, err := s.GetTasks(ctx, userID, limit, todayOffset, todayFilter)
	if err != nil {
		return result, err
	}
	result.Today = domain.TodayGroup{Tasks: tasks, Pagination: domain.Pagination{Limit: limit, Offset: todayOffset, Total: total}}
	overdueFilter := domain.TaskFilter{Status: &pending, SortBy: &sort, Order: &order, DeadlineBefore: &result.ServerNow}
	tasks, total, err = s.GetTasks(ctx, userID, limit, overdueOffset, overdueFilter)
	if err != nil {
		return result, err
	}
	result.Overdue = domain.TodayGroup{Tasks: tasks, Pagination: domain.Pagination{Limit: limit, Offset: overdueOffset, Total: total}}
	return result, nil
}
