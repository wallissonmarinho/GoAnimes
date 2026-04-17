package stremio

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPickStremioVideoReleasedISO_prefersSchedule(t *testing.T) {
	got := PickStremioVideoReleasedISO("2026-06-01T15:00:00.000Z", "2024-01-01T00:00:00.000Z")
	require.Equal(t, "2026-06-01T15:00:00.000Z", got)
}

func TestStremioMetaBehaviorHintsIfScheduled_futureRow(t *testing.T) {
	future := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339Nano)
	h := StremioMetaBehaviorHintsIfScheduled([]map[string]any{
		{"released": "2020-01-01T00:00:00.000Z"},
		{"released": future},
	})
	require.NotNil(t, h)
	require.Equal(t, true, h["hasScheduledVideos"])
}

func TestStremioMetaBehaviorHintsIfScheduled_noFuture(t *testing.T) {
	h := StremioMetaBehaviorHintsIfScheduled([]map[string]any{
		{"released": "2020-01-01T00:00:00.000Z"},
	})
	require.Nil(t, h)
}
