package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestGroupItemsByEpisode_oneRowMultipleReleases(t *testing.T) {
	sid := domain.SeriesStremioID("Test Show")
	items := []domain.CatalogItem{
		{ID: "goanimes:a", Name: "[Torrent] Test Show - 01 [720p CR WEB-DL AVC AAC][us][br]"},
		{ID: "goanimes:b", Name: "[Torrent] Test Show - 01 [1080p CR WEB-DL AVC AAC][us][br]"},
	}
	domain.AssignSeriesFields(items)
	require.Equal(t, sid, items[0].SeriesID)

	g := domain.GroupItemsByEpisode(items, sid)
	require.Len(t, g, 1)
	keys := domain.OrderedEpisodeKeys(g)
	require.Len(t, keys, 1)
	require.Len(t, g[keys[0]], 2)

	vid := domain.EpisodeVideoStremioID(sid, 1, 1, false)
	resolved := domain.ItemsForEpisodeVideoID(items, vid)
	require.Len(t, resolved, 2)
	require.Equal(t, "1080p", domain.ShortQualityHint(resolved[0].Name))
}

func TestEpisodeVideoStremioID_stable(t *testing.T) {
	sid := "goanimes:series:abc12345"
	v1 := domain.EpisodeVideoStremioID(sid, 1, 12, false)
	v2 := domain.EpisodeVideoStremioID(sid, 1, 12, false)
	require.Equal(t, v1, v2)
	require.True(t, domain.IsEpisodeVideoStremioID(v1))
}
