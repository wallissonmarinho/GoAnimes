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

func TestEpisodeTitlesFromStreamingList(t *testing.T) {
	m := domain.EpisodeTitlesFromStreamingList([]string{
		"Episode 1 - The Journey's End",
		"Episode 2 - It Didn't Have to Be Magic...",
		"garbage",
	})
	require.Equal(t, "The Journey's End", m[1])
	require.Equal(t, "It Didn't Have to Be Magic...", m[2])
	require.Len(t, m, 2)
}

func TestEpisodeListTitle_withAniListTitles(t *testing.T) {
	titles := map[int]string{3: "Killing Magic"}
	require.Equal(t, "E3 · Killing Magic", domain.EpisodeListTitle(3, false, titles, ""))
	require.Equal(t, "E3 · from torrent tail", domain.EpisodeListTitle(3, false, nil, "from torrent tail"))
	require.Equal(t, "E3", domain.EpisodeListTitle(3, false, nil, ""))
	require.Equal(t, "E3", domain.EpisodeListTitle(3, false, map[int]string{5: "other"}, ""))
	require.Equal(t, "Special", domain.EpisodeListTitle(0, true, titles, ""))
}

func TestTorrentReleaseEpisodeSuffix_eraiStyle(t *testing.T) {
	s := "[Torrent] Akuyaku Reijou wa Ringoku no Outaishi ni Dekiai sareru - 07 (HEVC) [1080p CR WEBRip HEVC AAC][us][br]"
	require.Contains(t, domain.TorrentReleaseEpisodeSuffix(s), "1080p")
	require.Contains(t, domain.TorrentReleaseEpisodeSuffix(s), "HEVC")
}
