package domain

import (
	"math"
	"time"
)

type UserStats struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	Points           *int64     `json:"points"`
	Level            *int64     `json:"level"`
	TotalPomodoros   *int64     `json:"total_pomodoros,omitzero"`
	TotalBurntTasks  *int64     `json:"total_burnt_tasks,omitzero"`
	CurrentStreak    *int64     `json:"current_streak,omitzero"`
	BestStreak       *int64     `json:"best_streak,omitzero"`
	LastActivityDate *time.Time `json:"last_activity_date,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at"`
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

type Achievement struct {
	ID          int64      `json:"id,omitempty"`
	UserID      int64      `json:"user_id,omitempty"`
	Code        string     `json:"code"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Icon        string     `json:"icon"`
	Points      int        `json:"points"`
	Unlocked    bool       `json:"unlocked"`
	UnlockedAt  *time.Time `json:"unlocked_at,omitempty"`
}

type DailyQuest struct {
	Code      string `json:"code"`
	Title     string `json:"title"`
	Icon      string `json:"icon"`
	Target    int    `json:"target"`
	Current   int    `json:"current"`
	Completed bool   `json:"completed"`
	RewardXP  int    `json:"reward_xp"`
}

type GamificationProfile struct {
	Level             int64        `json:"level"`
	Points            int64        `json:"points"`
	CurrentLevelXP    int64        `json:"current_level_xp"`
	NextLevelXP       int64        `json:"next_level_xp"`
	ProgressPercent   float64      `json:"progress_percent"`
	RankTitle         string       `json:"rank_title"`
	RankIcon          string       `json:"rank_icon"`
	Streak            int64        `json:"streak"`
	BestStreak        int64        `json:"best_streak"`
	StreakMultiplier  float64      `json:"streak_multiplier"`
	DailyQuests       []DailyQuest `json:"daily_quests"`
	UnlockedCount     int          `json:"unlocked_count"`
	TotalAchievements int          `json:"total_achievements"`
}

var AvailableAchievements = []Achievement{
	{Code: "first_step", Title: "Первый шаг", Description: "Завершить первую задачу", Icon: "🎯", Points: 25},
	{Code: "task_spree", Title: "Ударный темп", Description: "Завершить 10 задач", Icon: "⚡", Points: 50},
	{Code: "task_centurion", Title: "Центурион", Description: "Завершить 50 задач", Icon: "🏛️", Points: 200},
	{Code: "pomodoro_rookie", Title: "Свежий томат", Description: "Завершить первую сессию Pomodoro", Icon: "🍅", Points: 30},
	{Code: "pomodoro_guru", Title: "Дзен-фокус", Description: "Завершить 20 сессий Pomodoro", Icon: "🧘", Points: 150},
	{Code: "streak_three", Title: "Искра привычки", Description: "Серия активности 3 дня подряд", Icon: "🕯️", Points: 40},
	{Code: "streak_week", Title: "Неделя огня", Description: "Серия активности 7 дней подряд", Icon: "🔥", Points: 100},
	{Code: "streak_month", Title: "Железная воля", Description: "Серия активности 30 дней подряд", Icon: "👑", Points: 500},
	{Code: "phoenix", Title: "Пепел и Феникс", Description: "Сжечь задачу при срыве таймера и закрыть следующую", Icon: "💀", Points: 50},
	{Code: "night_owl", Title: "Полуночник", Description: "Завершить задачу в период с 00:00 до 05:00", Icon: "🦉", Points: 35},
	{Code: "early_bird", Title: "Ранняя пташка", Description: "Завершить задачу до 08:00 утра", Icon: "🌅", Points: 35},
	{Code: "pro_club", Title: "Золотой статус", Description: "Активация Премиум-подписки", Icon: "💎", Points: 100},
}

// CalculateLevel calculates the level, experience within current level, experience needed for next level, and progress percent.
func CalculateLevel(points int64) (level int64, currentLevelXP int64, nextLevelXP int64, percent float64, rankTitle string, rankIcon string) {
	if points < 0 {
		points = 0
	}

	level = 1
	remainingXP := points

	for {
		// XP required to pass the current level:
		// Level 1: 100 XP
		// Level 2: 150 XP
		// Level 3: 200 XP
		// ...
		needed := 100 + (level-1)*50
		if remainingXP < needed {
			currentLevelXP = remainingXP
			nextLevelXP = needed
			if nextLevelXP > 0 {
				percent = math.Round((float64(currentLevelXP)/float64(nextLevelXP))*1000) / 10
			}
			break
		}
		remainingXP -= needed
		level++
		if level >= 100 { // Cap at level 100
			currentLevelXP = needed
			nextLevelXP = needed
			percent = 100.0
			break
		}
	}

	switch {
	case level >= 20:
		rankTitle = "Легенда эффективности"
		rankIcon = "🌟"
	case level >= 10:
		rankTitle = "Гроссмейстер времени"
		rankIcon = "👑"
	case level >= 6:
		rankTitle = "Титан продуктивности"
		rankIcon = "💎"
	case level == 5:
		rankTitle = "Магистр Pomodoro"
		rankIcon = "🔥"
	case level == 4:
		rankTitle = "Мастер дедлайнов"
		rankIcon = "🎯"
	case level == 3:
		rankTitle = "Воин задач"
		rankIcon = "⚔️"
	case level == 2:
		rankTitle = "Искатель фокуса"
		rankIcon = "⚡"
	default:
		rankTitle = "Новичок"
		rankIcon = "🌱"
	}

	return
}

// GetStreakMultiplier returns XP multiplier based on consecutive streak days
func GetStreakMultiplier(streak int64) float64 {
	switch {
	case streak >= 7:
		return 2.0
	case streak >= 5:
		return 1.5
	case streak >= 3:
		return 1.2
	default:
		return 1.0
	}
}
