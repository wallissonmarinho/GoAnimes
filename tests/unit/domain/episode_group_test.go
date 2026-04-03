package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestMaxSeasonAmongSeriesItems(t *testing.T) {
	sid := domain.SeriesStremioID("Show")
	items := []domain.CatalogItem{
		{SeriesID: sid, Season: 1, Episode: 1},
		{SeriesID: sid, Season: 3, Episode: 2},
		{SeriesID: "other", Season: 99, Episode: 1},
	}
	require.Equal(t, 3, domain.MaxSeasonAmongSeriesItems(items, sid))
	require.Equal(t, 1, domain.MaxSeasonAmongSeriesItems(nil, sid))
	require.Equal(t, 1, domain.MaxSeasonAmongSeriesItems([]domain.CatalogItem{{SeriesID: sid, Season: 0, Episode: 1}}, sid))
}

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
	require.Equal(t, "Episódio 3 · Killing Magic", domain.EpisodeListTitle(3, false, titles, ""))
	require.Equal(t, "Episódio 3 · from torrent tail", domain.EpisodeListTitle(3, false, nil, "from torrent tail"))
	require.Equal(t, "Episódio 3", domain.EpisodeListTitle(3, false, nil, ""))
	require.Equal(t, "Episódio 3", domain.EpisodeListTitle(3, false, map[int]string{5: "other"}, ""))
	require.Equal(t, "Especial", domain.EpisodeListTitle(0, true, titles, ""))
}

func TestEpisodeListTitleForGroup_skipsTorrentNoise(t *testing.T) {
	group := []domain.CatalogItem{{Name: "[Torrent] X - 01 (HEVC) [1080p CR WEBRip HEVC AAC][us][br]"}}
	require.Equal(t, "Episódio 1", domain.EpisodeListTitleForGroup(1, false, nil, group))
}

func TestTorrentReleaseEpisodeSuffix_eraiStyle(t *testing.T) {
	s := "[Torrent] Akuyaku Reijou wa Ringoku no Outaishi ni Dekiai sareru - 07 (HEVC) [1080p CR WEBRip HEVC AAC][us][br]"
	require.Contains(t, domain.TorrentReleaseEpisodeSuffix(s), "1080p")
	require.Contains(t, domain.TorrentReleaseEpisodeSuffix(s), "HEVC")
}

func TestPreferTorrentOverMagnetReleases_sameBtih_prefersTorrent(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	items := []domain.CatalogItem{
		{ID: "goanimes:mag", Name: "[Magnet] Show - 01 [720p][br]", MagnetURL: "magnet:?xt=urn:btih:" + strings.ToUpper(hash) + "&dn=x", InfoHash: hash},
		{ID: "goanimes:tor", Name: "[Torrent] Show - 01 [720p][br]", TorrentURL: "https://example.com/x.torrent", InfoHash: hash},
	}
	out := domain.PreferTorrentOverMagnetReleases(items)
	require.Len(t, out, 1)
	require.Equal(t, "goanimes:tor", out[0].ID)
	require.NotEmpty(t, out[0].TorrentURL)
}

func TestPreferTorrentOverMagnetReleases_magnetOnly_kept(t *testing.T) {
	items := []domain.CatalogItem{
		{ID: "goanimes:m", Name: "[Magnet] Show - 01 [720p][br]", MagnetURL: "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=x"},
	}
	out := domain.PreferTorrentOverMagnetReleases(items)
	require.Len(t, out, 1)
	require.Equal(t, "goanimes:m", out[0].ID)
}

func TestPreferTorrentOverMagnetReleases_twoHashes_bothKept(t *testing.T) {
	items := []domain.CatalogItem{
		{ID: "a", Name: "[Magnet] Show - 01 [1080p][br]", MagnetURL: "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{ID: "b", Name: "[Magnet] Show - 01 [1080p HEVC][br]", MagnetURL: "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc"},
	}
	out := domain.PreferTorrentOverMagnetReleases(items)
	require.Len(t, out, 2)
}
