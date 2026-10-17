package get

import (
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestDeadlineFilters(t *testing.T) {
	value, err := parseFilter(httptest.NewRequest("GET", "/tasks?deadline_from=2026-10-14T00:00:00Z&deadline_before=2026-10-15T00:00:00Z&has_deadline=true", nil))
	require.NoError(t, err)
	require.NotNil(t, value.DeadlineFrom)
	require.NotNil(t, value.DeadlineBefore)
	require.True(t, *value.HasDeadline)
	for _, query := range []string{"deadline_from=no", "has_deadline=no", "has_deadline=false&deadline_from=2026-10-14T00:00:00Z", "deadline_from=2026-10-15T00:00:00Z&deadline_before=2026-10-14T00:00:00Z"} {
		_, err := parseFilter(httptest.NewRequest("GET", "/tasks?"+query, nil))
		require.Error(t, err)
	}
}
