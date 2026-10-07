package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateLevel(t *testing.T) {
	tests := []struct {
		points          int64
		expectedLevel   int64
		expectedCurrent int64
		expectedNext    int64
		expectedRank    string
		expectedIcon    string
	}{
		{points: 0, expectedLevel: 1, expectedCurrent: 0, expectedNext: 100, expectedRank: "Новичок", expectedIcon: "🌱"},
		{points: 50, expectedLevel: 1, expectedCurrent: 50, expectedNext: 100, expectedRank: "Новичок", expectedIcon: "🌱"},
		{points: 100, expectedLevel: 2, expectedCurrent: 0, expectedNext: 150, expectedRank: "Искатель фокуса", expectedIcon: "⚡"},
		{points: 140, expectedLevel: 2, expectedCurrent: 40, expectedNext: 150, expectedRank: "Искатель фокуса", expectedIcon: "⚡"},
		{points: 250, expectedLevel: 3, expectedCurrent: 0, expectedNext: 200, expectedRank: "Воин задач", expectedIcon: "⚔️"},
		{points: 450, expectedLevel: 4, expectedCurrent: 0, expectedNext: 250, expectedRank: "Мастер дедлайнов", expectedIcon: "🎯"},
		{points: 700, expectedLevel: 5, expectedCurrent: 0, expectedNext: 300, expectedRank: "Магистр Pomodoro", expectedIcon: "🔥"},
		{points: 1000, expectedLevel: 6, expectedCurrent: 0, expectedNext: 350, expectedRank: "Титан продуктивности", expectedIcon: "💎"},
		{points: 5000, expectedLevel: 13, expectedCurrent: 500, expectedNext: 700, expectedRank: "Гроссмейстер времени", expectedIcon: "👑"},
	}

	for _, tt := range tests {
		lvl, currentXP, nextXP, percent, rankTitle, rankIcon := CalculateLevel(tt.points)
		assert.Equal(t, tt.expectedLevel, lvl, "level for %d points", tt.points)
		assert.Equal(t, tt.expectedCurrent, currentXP, "current XP for %d points", tt.points)
		assert.Equal(t, tt.expectedNext, nextXP, "next XP for %d points", tt.points)
		assert.Equal(t, tt.expectedRank, rankTitle, "rank title for %d points", tt.points)
		assert.Equal(t, tt.expectedIcon, rankIcon, "rank icon for %d points", tt.points)
		assert.True(t, percent >= 0 && percent <= 100, "percent should be between 0 and 100")
	}
}

func TestGetStreakMultiplier(t *testing.T) {
	assert.Equal(t, 1.0, GetStreakMultiplier(0))
	assert.Equal(t, 1.0, GetStreakMultiplier(1))
	assert.Equal(t, 1.0, GetStreakMultiplier(2))
	assert.Equal(t, 1.2, GetStreakMultiplier(3))
	assert.Equal(t, 1.2, GetStreakMultiplier(4))
	assert.Equal(t, 1.5, GetStreakMultiplier(5))
	assert.Equal(t, 1.5, GetStreakMultiplier(6))
	assert.Equal(t, 2.0, GetStreakMultiplier(7))
	assert.Equal(t, 2.0, GetStreakMultiplier(14))
}

func TestAvailableAchievements(t *testing.T) {
	assert.NotEmpty(t, AvailableAchievements)
	codes := make(map[string]bool)
	for _, a := range AvailableAchievements {
		assert.NotEmpty(t, a.Code)
		assert.NotEmpty(t, a.Title)
		assert.NotEmpty(t, a.Description)
		assert.NotEmpty(t, a.Icon)
		assert.True(t, a.Points > 0)
		assert.False(t, codes[a.Code], "duplicate code %s", a.Code)
		codes[a.Code] = true
	}
}
