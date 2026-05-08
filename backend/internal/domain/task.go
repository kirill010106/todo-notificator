package domain

import "time"

const (
	TaskStatusPending  = "pending"
	TaskStatusDone     = "done"
	TaskStatusBurnt    = "burnt"
	TaskCompleteReward = 10
)

type Task struct {
	ID             int64      `json:"id"`
	CategoryID     *int64     `json:"category_id,omitzero"`
	UserID         int64      `json:"user_id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Deadline       *time.Time `json:"deadline,omitzero"`
	Status         string     `json:"status"`
	IsNotified     bool       `json:"is_notified"`
	PomodorosTaken int64      `json:"pomodoros_taken,omitempty"`
	ReminderAt     *time.Time `json:"reminder_at,omitzero"`
	RewardClaimed  bool       `json:"reward_claimed"`
}

type TaskUpdate struct {
	Title                   *string
	Description             *string
	Status                  *string
	ReminderAt              *time.Time `json:"reminder_at"`
	Deadline                *time.Time
	CategoryID              *int64
	IncrementPomodorosTaken bool
	RewardClaimed           bool
}

// TaskFilter holds optional filters for listing tasks.
type TaskFilter struct {
	Status *string // nil or "all" = no filter; must be one of ValidTaskStatuses
	Search *string // ILIKE search on title and description
}

// ValidTaskStatuses is the set of allowed status values for filtering.
var ValidTaskStatuses = map[string]bool{
	TaskStatusPending: true,
	TaskStatusDone:    true,
	TaskStatusBurnt:   true,
}
