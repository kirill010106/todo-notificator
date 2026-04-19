package domain

import "time"

type ActivityLog struct {
	ID          string         `json:"id"`
	UserID      int64          `json:"user_id"`
	Action      string         `json:"action"`
	EntityID    int64          `json:"entity_id"`
	DetailsJSON map[string]any `json:"details"`
	Timestamp   time.Time      `json:"timestamp"`
}