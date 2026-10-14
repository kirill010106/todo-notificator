package main

import (
	"context"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestLoggerEnvironmentLevels(t *testing.T) {
	for _, tc := range []struct {
		env   string
		debug bool
	}{{"local", true}, {"dev", true}, {"prod", false}, {"unknown", true}} {
		logger := setupLogger(tc.env)
		require.Equal(t, tc.debug, logger.Enabled(context.Background(), slog.LevelDebug), tc.env)
		require.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	}
}
