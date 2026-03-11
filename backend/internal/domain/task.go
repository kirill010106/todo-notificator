package domain

import "time"

const (
	TaskStatusPending = "pending"
	TaskStatusDone    = "done"
)

type Task struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Deadline    *time.Time `json:"deadline,omitzero"`
	Status      string     `json:"status"`
	IsNotified  bool       `json:"is_notified"`
}

type TaskUpdate struct {
	Title       *string
	Description *string
	Status      *string
	Deadline    *time.Time
}
