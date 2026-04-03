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
