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
	ID       int64
	Email    string
	Timezone string
}

type TaskWithUser struct {
	Task Task
	User User
}

type NotificationEvent struct {
	Task     Task
	User     User
	FireAt   time.Time
	Interval time.Duration
}

// ReminderDelivery carries a fencing token: an expired worker cannot acknowledge
// a newer lease or a reminder rescheduled by the user.
type ReminderDelivery struct {
	ID            int64
	LeaseToken    string
	Attempt       int
	ScheduledAt   time.Time
	OffsetMinutes *int64
	Task          Task
	User          User
}

type VerificationDelivery struct {
	ID         int64
	LeaseToken string
	Attempt    int
	Email      string
	Token      string
}
