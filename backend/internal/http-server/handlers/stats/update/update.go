package update

import (
	"time"

	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
)

type Request struct {
	Points          *int64     `json:"points"`
	Level           *int64     `json:"level"`
	TotalPomodoros  *int64     `json:"total_pomodoros,omitzero"`
	TotalBurntTasks *int64     `json:"total_burnt_tasks,omitzero"`
	CurrentStreak   *int64     `json:"current_streak,omitzero"`
	BestStreak      *int64     `json:"best_streak,omitzero"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

type Response struct {
	resp.Response
	UserID int64 `json:"category_id"`
}
