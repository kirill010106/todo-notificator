package domain

import "time"

type Task struct {
	ID          int64
	UserID      int64
	Title       string
	Description string
	Deadline    *time.Time
	Status      string
}

type User struct {
	ID    int64
	Email string
}

type TaskWithUser struct {
	Task Task
	User User
}

type NotificationEvent struct {
	Task Task
	User User
	FireAt time.Time
	Interval time.Duration
}
