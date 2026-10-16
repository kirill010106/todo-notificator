package domain

import (
	"fmt"
	"time"
)

// PlanReminders is shared by storage validation and deterministic rule tests.
// The returned list is the desired configuration; removed entries are retired.
func PlanReminders(existing []TaskReminder, inputs *[]ReminderInput, deadline *time.Time, deadlineChanged, premium bool, now time.Time) ([]TaskReminder, error) {
	byID := map[int64]TaskReminder{}
	for _, r := range existing {
		byID[r.ID] = r
	}
	desired := []ReminderInput{}
	if inputs == nil {
		for _, r := range existing {
			desired = append(desired, ReminderInput{ID: r.ID, OffsetMinutes: r.OffsetMinutes})
		}
	} else {
		desired = *inputs
	}
	result := make([]TaskReminder, 0, len(desired))
	ids := map[int64]bool{}
	times := map[string]bool{}
	changed := false
	for i, input := range desired {
		field := fmt.Sprintf("reminders.%d", i)
		old, found := byID[input.ID]
		if input.ID < 0 || (input.ID != 0 && (!found || ids[input.ID])) {
			return nil, ReminderError(field, "Напоминание не найдено в этой задаче или указано дважды")
		}
		ids[input.ID] = true
		r := old
		if !found {
			r = TaskReminder{Status: "scheduled"}
			changed = true
		}
		if found && old.LegacyAbsolute && input.OffsetMinutes == nil {
			// Preserve an imported exception, including its exact sub-minute instant.
		} else {
			if input.OffsetMinutes == nil || *input.OffsetMinutes < 0 || *input.OffsetMinutes > MaxReminderOffsetMinutes {
				return nil, ReminderError(field, "Укажи целое число минут от 0 до 52560000")
			}
			if deadline == nil {
				return nil, ReminderError("deadline", "Для напоминаний укажи срок или удали связанные напоминания")
			}
			offset := *input.OffsetMinutes
			modified := !found || old.LegacyAbsolute || old.OffsetMinutes == nil || *old.OffsetMinutes != offset
			if old.Status == "sent" && modified {
				return nil, ReminderError(field, "Отправленное напоминание нельзя перепланировать; добавь новое")
			}
			r.OffsetMinutes = &offset
			r.LegacyAbsolute = false
			if old.Status != "sent" {
				scheduled := deadline.Add(-time.Duration(offset) * time.Minute)
				moved := !found || modified || !old.ScheduledAt.Equal(scheduled)
				if moved && !scheduled.After(now) {
					return nil, ReminderError(field, "Это напоминание уже в прошлом. Измени время или удали его")
				}
				if moved {
					r.ScheduledAt = scheduled
					r.Status = "scheduled"
					r.SentAt = nil
					changed = true
				}
			}
			if modified {
				changed = true
			}
		}
		if times[r.ScheduledAt.UTC().Format(time.RFC3339Nano)] {
			return nil, ReminderError(field, "На это время уже есть напоминание")
		}
		times[r.ScheduledAt.UTC().Format(time.RFC3339Nano)] = true
		result = append(result, r)
	}
	if !premium && len(result) > 2 && changed {
		return nil, &ContractError{Code: "REMINDER_LIMIT_REACHED", Message: "В бесплатном тарифе можно настроить до двух напоминаний на задачу. Удали лишние или подключи Premium", Status: 403, Fields: map[string]string{"reminders": "Доступно до двух напоминаний на задачу"}}
	}
	return result, nil
}
