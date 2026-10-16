package domain

import "time"

const (
	TaskStatusPending  = "pending"
	TaskStatusDone     = "done"
	TaskStatusBurnt    = "burnt"
	TaskCompleteReward = 10
)

type Task struct {
	Reminders      []TaskReminder  `json:"reminders"`
	ReminderInputs []ReminderInput `json:"-"`
	ID             int64           `json:"id"`
	CategoryID     *int64          `json:"category_id,omitzero"`
	UserID         int64           `json:"user_id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Deadline       *time.Time      `json:"deadline,omitzero"`
	Status         string          `json:"status"`
	PomodorosTaken int64           `json:"pomodoros_taken,omitempty"`
	RewardClaimed  bool            `json:"reward_claimed"`
}

type TaskUpdate struct {
	Reminders               *[]ReminderInput
	Title                   *string
	Description             *string
	Status                  *string
	Deadline                *time.Time
	CategoryID              *int64
	IncrementPomodorosTaken bool
	RewardClaimed           bool
	ClearDeadline           bool
	ClearCategoryID         bool
}

// TaskFilter holds optional filters for listing tasks.
type TaskFilter struct {
	Status         *string // nil or "all" = no filter; must be one of ValidTaskStatuses
	Search         *string // ILIKE search on title and description
	CategoryID     *int64  // optional category filter
	SortBy         *string // "created_at" (default), "deadline"
	Order          *string // "desc" (default), "asc"
	DeadlineFrom   *time.Time
	DeadlineBefore *time.Time
	HasDeadline    *bool
}

// ValidTaskStatuses is the set of allowed status values for filtering.
var ValidTaskStatuses = map[string]bool{
	TaskStatusPending: true,
	TaskStatusDone:    true,
	TaskStatusBurnt:   true,
}
