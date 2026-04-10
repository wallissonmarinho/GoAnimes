package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestMergeAniListEnrichment_fillsEmpty(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{PosterURL: "https://p"}
	add := domain.AniListSeriesEnrichment{
		Description:       "Synopsis here",
		Genres:            []string{"Action"},
		StartYear:         2023,
		EpisodeTitleByNum: map[int]string{1: "Pilot"},
	}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, "https://p", out.PosterURL)
	require.Equal(t, "Synopsis here", out.Description)
	require.Equal(t, []string{"Action"}, out.Genres)
	require.Equal(t, 2023, out.StartYear)
	require.Equal(t, "Pilot", out.EpisodeTitleByNum[1])
}

func TestEnrichmentCouldUseJikan(t *testing.T) {
	require.True(t, domain.EnrichmentCouldUseJikan(domain.AniListSeriesEnrichment{}))
	require.True(t, domain.EnrichmentCouldUseJikan(domain.AniListSeriesEnrichment{
		Description:       "x",
		PosterURL:       "p",
		Genres:          []string{"A"},
		StartYear:       2020,
		EpisodeTitleByNum: map[int]string{1: "", 2: "  "},
	}), "only empty episode title placeholders should still allow Jikan")
	require.True(t, domain.EnrichmentCouldUseJikan(domain.AniListSeriesEnrichment{
		Description: "x",
		PosterURL:   "p",
		Genres:      []string{"A"},
		StartYear:   2020,
		// AniList often leaves episode titles empty → Jikan can fill.
		EpisodeTitleByNum: map[int]string{},
	}))
	require.False(t, domain.EnrichmentCouldUseJikan(domain.AniListSeriesEnrichment{
		Description:         "x",
		PosterURL:           "p",
		Genres:              []string{"A"},
		StartYear:           2020,
		EpisodeTitleByNum:   map[int]string{1: "Pilot"},
	}))
}

func TestMergeAniListEnrichment_keepsStored(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{Description: "A", Genres: []string{"Drama"}}
	add := domain.AniListSeriesEnrichment{Description: "B", Genres: []string{"Action"}}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, "A", out.Description)
	require.Equal(t, []string{"Drama"}, out.Genres)
}

func TestMergeAniListEnrichment_jikanDoesNotWipeNextAiring(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{
		NextAiringFromAniList: true,
		NextAiringUnix:        1700000000,
		NextAiringEpisode:     7,
	}
	add := domain.AniListSeriesEnrichment{
		Description: "from jikan",
		// NextAiringFromAniList false → must not clear AniList schedule
	}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, int64(1700000000), out.NextAiringUnix)
	require.Equal(t, 7, out.NextAiringEpisode)
	require.True(t, out.NextAiringFromAniList)
	require.Equal(t, "from jikan", out.Description)
}

func TestMergeAniListEnrichment_episodeTitleFillsEmptyPlaceholder(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{
		EpisodeTitleByNum: map[int]string{1: "Keep", 2: "", 3: "   "},
	}
	add := domain.AniListSeriesEnrichment{
		EpisodeTitleByNum: map[int]string{2: "From Kitsu", 3: "Three"},
	}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, "Keep", out.EpisodeTitleByNum[1])
	require.Equal(t, "From Kitsu", out.EpisodeTitleByNum[2])
	require.Equal(t, "Three", out.EpisodeTitleByNum[3])
}

func TestMergeAniListEnrichment_episodeThumbnailsFillGaps(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{
		EpisodeTitleByNum:     map[int]string{1: "A"},
		EpisodeThumbnailByNum: map[int]string{1: "https://u/1"},
	}
	add := domain.AniListSeriesEnrichment{
		EpisodeTitleByNum:     map[int]string{1: "B", 2: "C"},
		EpisodeThumbnailByNum: map[int]string{1: "https://other", 2: "https://u/2"},
	}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, "A", out.EpisodeTitleByNum[1])
	require.Equal(t, "C", out.EpisodeTitleByNum[2])
	require.Equal(t, "https://u/1", out.EpisodeThumbnailByNum[1])
	require.Equal(t, "https://u/2", out.EpisodeThumbnailByNum[2])
}

func TestMergeAniListEnrichment_tvdbSeriesID(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{}
	add := domain.AniListSeriesEnrichment{TvdbSeriesID: 452026}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, 452026, out.TvdbSeriesID)
	stored2 := domain.AniListSeriesEnrichment{TvdbSeriesID: 1}
	add2 := domain.AniListSeriesEnrichment{TvdbSeriesID: 2}
	out2 := domain.MergeAniListEnrichment(stored2, add2)
	require.Equal(t, 1, out2.TvdbSeriesID, "stored id wins")
}

func TestMergeAniListEnrichment_anidbAidAndFetchUnix(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{AniDBAid: 0, AniDBLastFetchedUnix: 100}
	add := domain.AniListSeriesEnrichment{AniDBAid: 19614, AniDBLastFetchedUnix: 200}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, 19614, out.AniDBAid)
	require.Equal(t, int64(200), out.AniDBLastFetchedUnix)
}

func TestMergeAniListEnrichment_anilistUpdatesNextAiring(t *testing.T) {
	stored := domain.AniListSeriesEnrichment{
		NextAiringFromAniList: true,
		NextAiringUnix:        100,
		NextAiringEpisode:     1,
	}
	add := domain.AniListSeriesEnrichment{
		NextAiringFromAniList: true,
		NextAiringUnix:        200,
		NextAiringEpisode:     2,
	}
	out := domain.MergeAniListEnrichment(stored, add)
	require.Equal(t, int64(200), out.NextAiringUnix)
	require.Equal(t, 2, out.NextAiringEpisode)
}
