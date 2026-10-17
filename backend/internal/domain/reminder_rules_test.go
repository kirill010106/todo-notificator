package domain

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestReminderRules(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(48 * time.Hour)
	ptr := func(v int64) *int64 { return &v }
	scheduled := TaskReminder{ID: 1, OffsetMinutes: ptr(60), ScheduledAt: deadline.Add(-time.Hour), Status: "scheduled"}
	sent := scheduled
	sent.Status = "sent"
	sent.ScheduledAt = now.Add(-time.Hour)
	legacy := TaskReminder{ID: 2, LegacyAbsolute: true, ScheduledAt: now.Add(-time.Hour), Status: "scheduled"}
	tests := []struct {
		name     string
		existing []TaskReminder
		inputs   []ReminderInput
		deadline *time.Time
		premium  bool
		field    string
	}{
		{"arbitrary offset", nil, []ReminderInput{{OffsetMinutes: ptr(1640)}}, &deadline, false, ""},
		{"zero offset", nil, []ReminderInput{{OffsetMinutes: ptr(0)}}, &deadline, false, ""},
		{"requires deadline", nil, []ReminderInput{{OffsetMinutes: ptr(0)}}, nil, false, "deadline"},
		{"requires offset", nil, []ReminderInput{{}}, &deadline, false, "reminders.0"},
		{"negative", nil, []ReminderInput{{OffsetMinutes: ptr(-1)}}, &deadline, false, "reminders.0"},
		{"overflow", nil, []ReminderInput{{OffsetMinutes: ptr(1 << 62)}}, &deadline, false, "reminders.0"},
		{"past", nil, []ReminderInput{{OffsetMinutes: ptr(4000)}}, &deadline, false, "reminders.0"},
		{"duplicate time", nil, []ReminderInput{{OffsetMinutes: ptr(0)}, {OffsetMinutes: ptr(0)}}, &deadline, false, "reminders.1"},
		{"foreign id", nil, []ReminderInput{{ID: 99, OffsetMinutes: ptr(0)}}, &deadline, false, "reminders.0"},
		{"negative id", nil, []ReminderInput{{ID: -1, OffsetMinutes: ptr(0)}}, &deadline, false, "reminders.0"},
		{"duplicate id", []TaskReminder{scheduled}, []ReminderInput{{ID: 1, OffsetMinutes: ptr(60)}, {ID: 1, OffsetMinutes: ptr(60)}}, &deadline, true, "reminders.1"},
		{"sent immutable", []TaskReminder{sent}, []ReminderInput{{ID: 1, OffsetMinutes: ptr(61)}}, &deadline, false, "reminders.0"},
		{"sent preserved", []TaskReminder{sent}, []ReminderInput{{ID: 1, OffsetMinutes: ptr(60)}}, &deadline, false, ""},
		{"legacy past preserved", []TaskReminder{legacy}, []ReminderInput{{ID: 2}}, nil, false, ""},
		{"legacy converted", []TaskReminder{legacy}, []ReminderInput{{ID: 2, OffsetMinutes: ptr(30)}}, &deadline, false, ""},
		{"free third", nil, []ReminderInput{{OffsetMinutes: ptr(0)}, {OffsetMinutes: ptr(1)}, {OffsetMinutes: ptr(2)}}, &deadline, false, "reminders"},
		{"premium arbitrary count", nil, []ReminderInput{{OffsetMinutes: ptr(0)}, {OffsetMinutes: ptr(1)}, {OffsetMinutes: ptr(2)}}, &deadline, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PlanReminders(tt.existing, &tt.inputs, tt.deadline, false, tt.premium, now)
			if tt.field != "" {
				var contract *ContractError
				require.ErrorAs(t, err, &contract)
				require.Contains(t, contract.Fields, tt.field)
			} else {
				require.NoError(t, err)
				require.Len(t, result, len(tt.inputs))
			}
		})
	}
	t.Run("downgrade unchanged and deletion", func(t *testing.T) {
		existing := []TaskReminder{}
		for i := int64(0); i < 4; i++ {
			r := scheduled
			r.ID = i + 1
			r.OffsetMinutes = ptr(i)
			r.ScheduledAt = deadline.Add(-time.Duration(i) * time.Minute)
			existing = append(existing, r)
		}
		result, err := PlanReminders(existing, nil, &deadline, false, false, now)
		require.NoError(t, err)
		require.Len(t, result, 4)
		inputs := []ReminderInput{{ID: 1, OffsetMinutes: ptr(0)}, {ID: 2, OffsetMinutes: ptr(1)}, {ID: 3, OffsetMinutes: ptr(2)}}
		_, err = PlanReminders(existing, &inputs, &deadline, false, false, now)
		require.NoError(t, err)
		moved := deadline.Add(time.Hour)
		_, err = PlanReminders(existing, nil, &moved, true, false, now)
		require.Error(t, err)
	})
	t.Run("omitted shifts unsent only", func(t *testing.T) {
		moved := deadline.Add(time.Hour)
		rows, err := PlanReminders([]TaskReminder{scheduled}, nil, &moved, true, false, now)
		require.NoError(t, err)
		require.Equal(t, deadline, rows[0].ScheduledAt)
		rows, err = PlanReminders([]TaskReminder{sent}, nil, &moved, true, false, now)
		require.NoError(t, err)
		require.Equal(t, sent.ScheduledAt, rows[0].ScheduledAt)
	})
}
func TestReminderRequestFields(t *testing.T) {
	for _, data := range []string{`{"reminder_at":null}`, `{"is_notified":false}`, `{"reminders":null}`, `invalid`} {
		require.Error(t, ValidateReminderFields([]byte(data)))
	}
	require.NoError(t, ValidateReminderFields([]byte(`{"reminders":[]}`)))
	for _, data := range []string{`{"offset_minutes":1.5}`, `{"scheduled_at":"tomorrow"}`, `{"status":"sent"}`} {
		var input ReminderInput
		require.Error(t, json.Unmarshal([]byte(data), &input))
	}
	var input ReminderInput
	require.NoError(t, json.Unmarshal([]byte(`{"id":2,"offset_minutes":0}`), &input))
	require.Equal(t, int64(0), *input.OffsetMinutes)
}
