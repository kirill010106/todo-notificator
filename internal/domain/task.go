package domain

import "time"

const (
	TaskStatusPending = "pending"
	TaskStatusDone    = "done"
)

type Task struct {
	ID          int64
	UserID      int
	Title       string
	Description string
	Deadline    *time.Time
	Status      string
	IsNotified  bool
}

type TaskUpdate struct {
	Title       *string
	Description *string
	Status      *string
	Deadline    *time.Time
}
