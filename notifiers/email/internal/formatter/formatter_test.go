package formatter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatDeadlineUsesUserTimezone(t *testing.T) {
	deadline := time.Date(2026, 10, 18, 15, 0, 0, 0, time.UTC)
	require.Equal(t, "18.10.2026 at 18:00", formatDeadline(&deadline, "Europe/Moscow"))
	require.Equal(t, "18.10.2026 at 15:00", formatDeadline(&deadline, "Local"))
}
