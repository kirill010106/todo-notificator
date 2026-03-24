package domain

import "time"

type PomodoroSessions struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	TaskID          int64      `json:"task_id"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	DurationMinutes int64      `json:"duration_minutes"`
	BreaksUsed      int64      `json:"breaks_used,omitzero"`
	CreatedAt       *time.Time `json:"created_at"`
}
