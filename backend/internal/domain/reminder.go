package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

func ValidateReminderFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"reminder_at", "is_notified"} {
		if _, ok := fields[key]; ok {
			return fmt.Errorf("поле %s больше не поддерживается; используй reminders", key)
		}
	}
	if value, ok := fields["reminders"]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return ReminderError("reminders", "Укажи массив: [] удаляет все напоминания")
	}
	if value, ok := fields["reminders"]; ok {
		var rows []json.RawMessage
		if err := json.Unmarshal(value, &rows); err != nil {
			return ReminderError("reminders", "Укажи список напоминаний")
		}
		for i, row := range rows {
			var input ReminderInput
			if bytes.Equal(bytes.TrimSpace(row), []byte("null")) {
				return ReminderError(fmt.Sprintf("reminders.%d", i), "Укажи настройки напоминания")
			}
			if err := json.Unmarshal(row, &input); err != nil {
				return ReminderError(fmt.Sprintf("reminders.%d", i), "Укажи id и целое неотрицательное offset_minutes; дата и состояние отправки рассчитываются сервером")
			}
		}
	}
	return nil
}

// MaxReminderOffsetMinutes is a numerical/date-safety boundary, not a tariff quota.
const MaxReminderOffsetMinutes int64 = 52560000 // 100 * 365 days

type TaskReminder struct {
	ID             int64      `json:"id"`
	OffsetMinutes  *int64     `json:"offset_minutes"`
	ScheduledAt    time.Time  `json:"scheduled_at"`
	Status         string     `json:"status"`
	SentAt         *time.Time `json:"sent_at"`
	LegacyAbsolute bool       `json:"legacy_absolute"`
}

type ReminderInput struct {
	ID            int64  `json:"id,omitempty"`
	OffsetMinutes *int64 `json:"offset_minutes"`
}

// Reject delivery/read-only fields instead of silently accepting an independent date.
func (r *ReminderInput) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range fields {
		if key != "id" && key != "offset_minutes" {
			return fmt.Errorf("reminders: поле %s доступно только для чтения", key)
		}
	}
	type plain ReminderInput
	return json.Unmarshal(data, (*plain)(r))
}

func ReminderError(field, message string) *ContractError {
	return &ContractError{Code: "REMINDER_VALIDATION_ERROR", Message: message, Status: 400, Fields: map[string]string{field: message}}
}
