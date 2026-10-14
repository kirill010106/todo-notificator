package domain

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAfterCommitDefersTransactionEffects(t *testing.T) {
	calls := 0
	AfterCommit(context.Background(), func() { calls++ })
	require.Equal(t, 1, calls)
	var pending []func()
	ctx := WithAfterCommit(context.Background(), &pending)
	AfterCommit(ctx, func() { calls++ })
	require.Equal(t, 1, calls)
	require.Len(t, pending, 1)
	pending[0]()
	require.Equal(t, 2, calls)
}
func TestLevelBoundariesAndCap(t *testing.T) {
	for _, tc := range []struct {
		points, level, xp int64
		percent           float64
	}{
		{-1, 1, 0, 0}, {99, 1, 99, 99}, {100, 2, 0, 0}, {250, 3, 0, 0}, {450, 4, 0, 0}, {700, 5, 0, 0}, {1000, 6, 0, 0},
		{2700, 10, 0, 0}, {10450, 20, 0, 0},
	} {
		level, xp, _, percent, title, icon := CalculateLevel(tc.points)
		require.Equal(t, tc.level, level, "points=%d", tc.points)
		require.Equal(t, tc.xp, xp)
		require.Equal(t, tc.percent, percent)
		require.NotEmpty(t, title)
		require.NotEmpty(t, icon)
	}
	level, _, _, percent, _, _ := CalculateLevel(1000000000)
	require.Equal(t, int64(100), level)
	require.Equal(t, 100.0, percent)
	for _, tc := range []struct {
		days int64
		want float64
	}{{-1, 1}, {2, 1}, {3, 1.2}, {4, 1.2}, {5, 1.5}, {6, 1.5}, {7, 2}, {365, 2}} {
		require.Equal(t, tc.want, GetStreakMultiplier(tc.days))
	}
}
