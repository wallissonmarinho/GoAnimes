package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestMergeSeriesEnrichment(t *testing.T) {
	t.Run("fillsEmpty", func(t *testing.T) {
		stored := domain.SeriesEnrichment{PosterURL: "https://p"}
		add := domain.SeriesEnrichment{
			Description:       "Synopsis here",
			Genres:            []string{"Action"},
			StartYear:         2023,
			EpisodeTitleByNum: map[int]string{1: "Pilot"},
		}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, "https://p", out.PosterURL)
		require.Equal(t, "Synopsis here", out.Description)
		require.Equal(t, []string{"Action"}, out.Genres)
		require.Equal(t, 2023, out.StartYear)
		require.Equal(t, "Pilot", out.EpisodeTitleByNum[1])
	})

	t.Run("keepsStoredDescriptionAndGenres", func(t *testing.T) {
		stored := domain.SeriesEnrichment{Description: "A", Genres: []string{"Drama"}}
		add := domain.SeriesEnrichment{Description: "B", Genres: []string{"Action"}}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, "A", out.Description)
		require.Equal(t, []string{"Drama"}, out.Genres)
	})

	t.Run("doesNotWipeNextAiring", func(t *testing.T) {
		stored := domain.SeriesEnrichment{
			NextAiringFromAniList: true,
			NextAiringUnix:        1700000000,
			NextAiringEpisode:     7,
		}
		add := domain.SeriesEnrichment{
			Description: "from provider",
			// NextAiringFromAniList false → must not clear stored schedule
		}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, int64(1700000000), out.NextAiringUnix)
		require.Equal(t, 7, out.NextAiringEpisode)
		require.True(t, out.NextAiringFromAniList)
		require.Equal(t, "from provider", out.Description)
	})

	t.Run("episodeTitleFillsEmptyPlaceholder", func(t *testing.T) {
		stored := domain.SeriesEnrichment{
			EpisodeTitleByNum: map[int]string{1: "Keep", 2: "", 3: "   "},
		}
		add := domain.SeriesEnrichment{
			EpisodeTitleByNum: map[int]string{2: "From source", 3: "Three"},
		}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, "Keep", out.EpisodeTitleByNum[1])
		require.Equal(t, "From source", out.EpisodeTitleByNum[2])
		require.Equal(t, "Three", out.EpisodeTitleByNum[3])
	})

	t.Run("episodeThumbnailsFillGaps", func(t *testing.T) {
		stored := domain.SeriesEnrichment{
			EpisodeTitleByNum:     map[int]string{1: "A"},
			EpisodeThumbnailByNum: map[int]string{1: "https://u/1"},
		}
		add := domain.SeriesEnrichment{
			EpisodeTitleByNum:     map[int]string{1: "B", 2: "C"},
			EpisodeThumbnailByNum: map[int]string{1: "https://other", 2: "https://u/2"},
		}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, "A", out.EpisodeTitleByNum[1])
		require.Equal(t, "C", out.EpisodeTitleByNum[2])
		require.Equal(t, "https://u/1", out.EpisodeThumbnailByNum[1])
		require.Equal(t, "https://u/2", out.EpisodeThumbnailByNum[2])
	})

	t.Run("tvdbSeriesID", func(t *testing.T) {
		stored := domain.SeriesEnrichment{}
		add := domain.SeriesEnrichment{TvdbSeriesID: 452026}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, 452026, out.TvdbSeriesID)
		stored2 := domain.SeriesEnrichment{TvdbSeriesID: 1}
		add2 := domain.SeriesEnrichment{TvdbSeriesID: 2}
		out2 := domain.MergeSeriesEnrichment(stored2, add2)
		require.Equal(t, 1, out2.TvdbSeriesID, "stored id wins")
	})

	t.Run("updatesNextAiringWhenFlagged", func(t *testing.T) {
		stored := domain.SeriesEnrichment{
			NextAiringFromAniList: true,
			NextAiringUnix:        100,
			NextAiringEpisode:     1,
		}
		add := domain.SeriesEnrichment{
			NextAiringFromAniList: true,
			NextAiringUnix:        200,
			NextAiringEpisode:     2,
		}
		out := domain.MergeSeriesEnrichment(stored, add)
		require.Equal(t, int64(200), out.NextAiringUnix)
		require.Equal(t, 2, out.NextAiringEpisode)
	})
}
