package domain

import "time"

const (
	TaskStatusPending = "pending"
	TaskStatusDone    = "done"
)

type Task struct {
	ID          int64      `json:"id"`
	CategoryID  *int64     `json:"category_id,omitzero"`
	UserID      int64      `json:"user_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Deadline    *time.Time `json:"deadline,omitzero"`
	Status      string     `json:"status"`
	IsNotified  bool       `json:"is_notified"`
	ReminderAt  *time.Time `json:"reminder_at,omitzero"`
}

type TaskUpdate struct {
	Title       *string
	Description *string
	Status      *string
	ReminderAt  *time.Time `json:"reminder_at"`
	Deadline    *time.Time
	CategoryID  *int64
}
