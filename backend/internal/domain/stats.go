package domain

import "time"

type UserStats struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	Points          *int64     `json:"points"`
	Level           *int64     `json:"level"`
	TotalPomodoros  *int64     `json:"total_pomodoros,omitzero"`
	TotalBurntTasks *int64     `json:"total_burnt_tasks,omitzero"`
	CurrentStreak   *int64     `json:"current_streak,omitzero"`
	BestStreak      *int64     `json:"best_streak,omitzero"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

type UserStatsUpdate struct {
	Points          *int64 `json:"points"`
	Level           *int64 `json:"level"`
	TotalPomodoros  *int64 `json:"total_pomodoros,omitzero"`
	TotalBurntTasks *int64 `json:"total_burnt_tasks,omitzero"`
	CurrentStreak   *int64 `json:"current_streak,omitzero"`
	BestStreak      *int64 `json:"best_streak,omitzero"`
}

type StatsDelta struct {
	PointsDelta     int
	PomodorosDelta  int
	BurntTasksDelta int
	ResetStreak     bool
	IncrementStreak bool
}
