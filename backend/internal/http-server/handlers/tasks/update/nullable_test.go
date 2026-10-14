package update

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNullablePatchDistinguishesMissingAndNull(t *testing.T) {
	var omitted, cleared Request
	require.NoError(t, json.Unmarshal([]byte(`{"title":"Changed"}`), &omitted))
	require.False(t, omitted.ToDomain().ClearDeadline)
	require.NoError(t, json.Unmarshal([]byte(`{"deadline":null,"reminder_at":null,"category_id":null}`), &cleared))
	require.False(t, cleared.IsEmpty())
	require.True(t, cleared.ToDomain().ClearDeadline)
	require.True(t, cleared.ToDomain().ClearReminderAt)
	require.True(t, cleared.ToDomain().ClearCategoryID)
}
