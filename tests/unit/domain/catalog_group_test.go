package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestParseEraiReleaseTitle(t *testing.T) {
	sn, ep, sp, ok := domain.ParseEraiReleaseTitle("[Torrent] Chitose-kun wa Ramune Bin no Naka - 13 [720p CR WEB-DL AVC AAC][us][br]")
	require.True(t, ok)
	require.Equal(t, "Chitose-kun wa Ramune Bin no Naka", sn)
	require.Equal(t, 13, ep)
	require.False(t, sp)

	sn, ep, sp, ok = domain.ParseEraiReleaseTitle("[Torrent] Isekai Foo - Special [SD CR WEB-DL AVC AAC][us][br]")
	require.True(t, ok)
	require.Equal(t, "Isekai Foo", sn)
	require.Equal(t, 0, ep)
	require.True(t, sp)

	sn, ep, sp, ok = domain.ParseEraiReleaseTitle("[Torrent] Fangkai Nage Nuwu - 05v2 (Chinese Audio) [1080p CR WEB-DL AVC AAC][us][br]")
	require.True(t, ok)
	require.Equal(t, "Fangkai Nage Nuwu", sn)
	require.Equal(t, 5, ep)
	require.False(t, sp)

	sn, ep, sp, ok = domain.ParseEraiReleaseTitle("[Magnet] Champignon no Majo - 12 (HEVC) [1080p CR WEBRip HEVC AAC][us][br]")
	require.True(t, ok)
	require.Equal(t, "Champignon no Majo", sn)
	require.Equal(t, 12, ep)
	require.False(t, sp)

	sn2, ep2, _, ok2 := domain.ParseEraiReleaseTitle("[Torrent] Champignon no Majo - 12 (HEVC) [1080p CR WEBRip HEVC AAC][us][br]")
	require.True(t, ok2)
	require.Equal(t, sn, sn2, "magnet and torrent rows must share the same series name")
	require.Equal(t, ep, ep2)

	_, _, _, ok = domain.ParseEraiReleaseTitle("no pattern here")
	require.False(t, ok)
}

func TestEraiSeasonFromSeriesName(t *testing.T) {
	require.Equal(t, 1, domain.EraiSeasonFromSeriesName("Some Anime"))
	require.Equal(t, 2, domain.EraiSeasonFromSeriesName("Dorohedoro Season 2"))
	require.Equal(t, 3, domain.EraiSeasonFromSeriesName("Foo 3rd Season"))
	require.Equal(t, 2, domain.EraiSeasonFromSeriesName("Bar S2"))
	require.Equal(t, 2, domain.EraiSeasonFromSeriesName("Baz Part 2"))
}

func TestIsBatchReleaseTitle(t *testing.T) {
	require.True(t, domain.IsBatchReleaseTitle("[Torrent] Meitantei Precure - 01 ~ 10 [1080p][Batch]"))
	require.True(t, domain.IsBatchReleaseTitle("[Magnet] Foo - 01 ~ 03 [720p]"))
	require.False(t, domain.IsBatchReleaseTitle("[Torrent] Foo - 11 [1080p][Airing]"))
}

func TestDropBatchCatalogItems(t *testing.T) {
	items := []domain.CatalogItem{
		{Name: "[Torrent] Foo - 01 ~ 10 [Batch]"},
		{Name: "[Torrent] Foo - 11 [Airing]"},
	}
	out, dropped := domain.DropBatchCatalogItems(items)
	require.Equal(t, 1, dropped)
	require.Len(t, out, 1)
	require.Equal(t, "[Torrent] Foo - 11 [Airing]", out[0].Name)
}

func TestApplySeriesEnrichmentToSeries(t *testing.T) {
	snap := &domain.CatalogSnapshot{
		Series: []domain.CatalogSeries{
			{ID: "goanimes:series:aa", Name: "A", Poster: "http://placeholder"},
		},
		SeriesEnrichmentBySeriesID: map[string]domain.SeriesEnrichment{
			"goanimes:series:aa": {
				PosterURL:      "https://cdn.anilist/1.jpg",
				Description:    "Synopsis",
				TitlePreferred: "A Latin",
				TitleNative:    "エー",
				Genres:         []string{"Action", "Fantasy"},
				StartYear:      2024,
			},
		},
	}
	domain.ApplySeriesEnrichmentToSeries(snap)
	require.Equal(t, "https://cdn.anilist/1.jpg", snap.Series[0].Poster)
	require.Equal(t, "A Latin", snap.Series[0].Name)
	require.Equal(t, "Synopsis", snap.Series[0].Description)
	require.Equal(t, []string{"Ação", "Fantasia"}, snap.Series[0].Genres)
	require.Equal(t, "2024-", snap.Series[0].ReleaseInfo)
}

func TestExternalSearchQueryFromItems(t *testing.T) {
	sid := domain.SeriesStremioID("RSS Title Here")
	items := []domain.CatalogItem{
		{SeriesID: "other", SeriesName: "ignore"},
		{SeriesID: sid, SeriesName: "RSS Title Here"},
	}
	require.Equal(t, "RSS Title Here", domain.ExternalSearchQueryFromItems(items, sid))
	require.Equal(t, "", domain.ExternalSearchQueryFromItems(items, "missing"))
}

func TestEnsureSnapshotGrouped(t *testing.T) {
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "goanimes:a", Name: "[Torrent] Zeta Show - 02 [1080p][br]", Released: "2026-04-01"},
			{ID: "goanimes:b", Name: "[Torrent] Alpha Show - 01 [720p][br]", Released: "2026-04-03"},
		},
	}
	domain.EnsureSnapshotGrouped(snap)
	require.Len(t, snap.Series, 2)
	// Catalog order: newest RSS pubDate first (Alpha newer than Zeta).
	require.Equal(t, "Alpha Show", snap.Series[0].Name)
	require.Equal(t, "Zeta Show", snap.Series[1].Name)
	require.NotEmpty(t, snap.Items[0].SeriesID)
}

func TestBuildSeriesList_ordersByNewestRSSDate(t *testing.T) {
	items := []domain.CatalogItem{
		{SeriesID: "s-old", SeriesName: "Old Anime", Released: "2026-01-01"},
		{SeriesID: "s-new", SeriesName: "New Anime", Released: "2026-04-03T14:36:02Z"},
		{SeriesID: "s-mid", SeriesName: "Mid Anime", Released: "2026-03-15"},
	}
	list := domain.BuildSeriesList(items)
	require.Len(t, list, 3)
	require.Equal(t, "New Anime", list[0].Name)
	require.Equal(t, "Mid Anime", list[1].Name)
	require.Equal(t, "Old Anime", list[2].Name)
}

func TestBuildSeriesList_sameCalendarDay_ordersByWallClock(t *testing.T) {
	items := []domain.CatalogItem{
		{SeriesID: "s-late", SeriesName: "Late Show", Released: "2026-04-04T16:01:13Z"},
		{SeriesID: "s-early", SeriesName: "Early Show", Released: "2026-04-04T15:57:50Z"},
	}
	list := domain.BuildSeriesList(items)
	require.Len(t, list, 2)
	require.Equal(t, "Late Show", list[0].Name)
	require.Equal(t, "Early Show", list[1].Name)
}

func TestLatestReleased_sameCalendarDay_prefersLaterInstant(t *testing.T) {
	items := []domain.CatalogItem{
		{Released: "2026-04-04T15:57:50Z"},
		{Released: "2026-04-04T16:01:13Z"},
	}
	require.Equal(t, "2026-04-04T16:01:13Z", domain.LatestReleased(items))
}
