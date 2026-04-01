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
