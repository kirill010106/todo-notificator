package domain

import (
	"context"
	"time"
)

type afterCommitKey struct{}

func WithAfterCommit(ctx context.Context, callbacks *[]func()) context.Context {
	return context.WithValue(ctx, afterCommitKey{}, callbacks)
}
func AfterCommit(ctx context.Context, callback func()) {
	if callbacks, ok := ctx.Value(afterCommitKey{}).(*[]func()); ok {
		*callbacks = append(*callbacks, callback)
	} else {
		callback()
	}
}

type FocusState struct {
	ServerNow            time.Time        `json:"server_now"`
	Phase                string           `json:"phase"`
	Session              *PomodoroSession `json:"session"`
	PhaseEndsAt          *time.Time       `json:"phase_ends_at"`
	RemainingWorkSeconds int              `json:"remaining_work_seconds"`
	CompletedInCycle     int              `json:"completed_in_cycle"`
	AvailableActions     []string         `json:"available_actions"`
}

type ActionResult struct {
	PomodoroCounted bool    `json:"pomodoro_counted"`
	TaskCompleted   bool    `json:"task_completed"`
	ChangedTaskIDs  []int64 `json:"changed_task_ids"`
	SkippedTaskIDs  []int64 `json:"skipped_task_ids"`
}

type ActionRewards struct {
	TaskXP        int `json:"task_xp"`
	PomodoroXP    int `json:"pomodoro_xp"`
	AchievementXP int `json:"achievement_xp"`
	QuestXP       int `json:"quest_xp"`
	PenaltyXP     int `json:"penalty_xp"`
}

type ActionOutcome struct {
	Result  ActionResult  `json:"result"`
	Rewards ActionRewards `json:"rewards"`
}

type OperationResponse struct {
	Body   []byte
	Status int
}

type ContractError struct {
	Code    string
	Message string
	Status  int
}

func (e *ContractError) Error() string { return e.Message }

type TodayGroup struct {
	Tasks      []Task     `json:"tasks"`
	Pagination Pagination `json:"pagination"`
}
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
type TodayState struct {
	ServerNow time.Time  `json:"server_now"`
	Timezone  string     `json:"timezone"`
	Today     TodayGroup `json:"today"`
	Overdue   TodayGroup `json:"overdue"`
}
