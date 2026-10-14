package slogpretty

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
	"time"
)

func TestPrettyLogLevelsAndContext(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		t.Run(level.String(), func(t *testing.T) {
			var output bytes.Buffer
			handler := PrettyHandlerOptions{SlogOpts: &slog.HandlerOptions{Level: slog.LevelInfo}}.NewPrettyHandler(&output)
			require.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
			require.True(t, handler.Enabled(context.Background(), slog.LevelError))
			record := slog.NewRecord(time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC), level, "request failed", 0)
			record.AddAttrs(slog.Int("status", 503))
			require.NoError(t, handler.WithAttrs([]slog.Attr{slog.String("request_id", "request-1")}).Handle(context.Background(), record))
			require.Contains(t, output.String(), level.String()+":")
			require.Contains(t, output.String(), `"status": 503`)
			require.Contains(t, output.String(), `"request_id": "request-1"`)
			require.Contains(t, output.String(), "request failed")
			require.NoError(t, handler.Handle(context.Background(), slog.NewRecord(time.Now(), level, "no fields", 0)))
		})
	}
}
func TestPrettyLogRejectsUnsupportedValues(t *testing.T) {
	var output bytes.Buffer
	handler := PrettyHandlerOptions{}.NewPrettyHandler(&output)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "bad value", 0)
	record.AddAttrs(slog.Any("unsupported", make(chan int)))
	require.Error(t, handler.Handle(context.Background(), record))
	require.Empty(t, output.String())
}
