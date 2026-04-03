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

func TestApplyAniListEnrichmentToSeries(t *testing.T) {
	snap := &domain.CatalogSnapshot{
		Series: []domain.CatalogSeries{
			{ID: "goanimes:series:aa", Name: "A", Poster: "http://placeholder"},
		},
		AniListBySeries: map[string]domain.AniListSeriesEnrichment{
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
	domain.ApplyAniListEnrichmentToSeries(snap)
	require.Equal(t, "https://cdn.anilist/1.jpg", snap.Series[0].Poster)
	require.Equal(t, "A Latin", snap.Series[0].Name)
	require.Equal(t, "Synopsis", snap.Series[0].Description)
	require.Equal(t, []string{"Ação", "Fantasia"}, snap.Series[0].Genres)
	require.Equal(t, "2024-", snap.Series[0].ReleaseInfo)
}

func TestAniListSearchQueryFromItems(t *testing.T) {
	sid := domain.SeriesStremioID("RSS Title Here")
	items := []domain.CatalogItem{
		{SeriesID: "other", SeriesName: "ignore"},
		{SeriesID: sid, SeriesName: "RSS Title Here"},
	}
	require.Equal(t, "RSS Title Here", domain.AniListSearchQueryFromItems(items, sid))
	require.Equal(t, "", domain.AniListSearchQueryFromItems(items, "missing"))
}

func TestEnsureSnapshotGrouped(t *testing.T) {
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "goanimes:a", Name: "[Torrent] Zeta Show - 02 [1080p][br]"},
			{ID: "goanimes:b", Name: "[Torrent] Alpha Show - 01 [720p][br]"},
		},
	}
	domain.EnsureSnapshotGrouped(snap)
	require.Len(t, snap.Series, 2)
	require.Equal(t, "Alpha Show", snap.Series[0].Name)
	require.Equal(t, "Zeta Show", snap.Series[1].Name)
	require.NotEmpty(t, snap.Items[0].SeriesID)
}
