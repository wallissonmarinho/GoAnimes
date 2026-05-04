package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

func TestParseSeriesID(t *testing.T) {
	// Valid series ID format: tmdb:123:2 (tmdb, season 2)
	tmdbID, season, ok := domain.ParseSeriesID("tmdb:123:2")
	require.True(t, ok)
	require.Equal(t, 123, tmdbID)
	require.Equal(t, 2, season)

	// Valid with different season
	tmdbID, season, ok = domain.ParseSeriesID("tmdb:456:1")
	require.True(t, ok)
	require.Equal(t, 456, tmdbID)
	require.Equal(t, 1, season)

	// Invalid format
	_, _, ok = domain.ParseSeriesID("invalid")
	require.False(t, ok)

	// Empty string
	_, _, ok = domain.ParseSeriesID("")
	require.False(t, ok)

	// Missing parts
	_, _, ok = domain.ParseSeriesID("tmdb:123")
	require.False(t, ok)
}

func TestParseEpisodeID(t *testing.T) {
	// Valid episode ID format: tmdb:123:2:5 (tmdb, season 2, episode 5)
	tmdbID, season, episode, ok := domain.ParseEpisodeID("tmdb:123:2:5")
	require.True(t, ok)
	require.Equal(t, 123, tmdbID)
	require.Equal(t, 2, season)
	require.Equal(t, 5, episode)

	// Different episode
	tmdbID, season, episode, ok = domain.ParseEpisodeID("tmdb:999:1:12")
	require.True(t, ok)
	require.Equal(t, 999, tmdbID)
	require.Equal(t, 1, season)
	require.Equal(t, 12, episode)

	// Invalid format
	_, _, _, ok = domain.ParseEpisodeID("invalid")
	require.False(t, ok)

	// Empty string
	_, _, _, ok = domain.ParseEpisodeID("")
	require.False(t, ok)

	// Missing parts
	_, _, _, ok = domain.ParseEpisodeID("tmdb:123:2")
	require.False(t, ok)
}

func TestSeriesStremioID(t *testing.T) {
	// Format: tmdb:TMDBID:season
	id := domain.SeriesStremioID(123, 2)
	require.Equal(t, "tmdb:123:2", id)

	// Different IDs
	id = domain.SeriesStremioID(456, 1)
	require.Equal(t, "tmdb:456:1", id)

	// Season 0
	id = domain.SeriesStremioID(789, 0)
	require.Equal(t, "tmdb:789:0", id)
}

func TestEpisodeStremioID(t *testing.T) {
	// Format: tmdb:TMDBID:season:episode
	id := domain.EpisodeStremioID(123, 2, 5)
	require.Equal(t, "tmdb:123:2:5", id)

	// Different values
	id = domain.EpisodeStremioID(999, 1, 12)
	require.Equal(t, "tmdb:999:1:12", id)

	// Episode 1
	id = domain.EpisodeStremioID(111, 3, 1)
	require.Equal(t, "tmdb:111:3:1", id)
}

func TestRoundTripSeriesID(t *testing.T) {
	// Series ID round trip: generate and parse
	original := domain.SeriesStremioID(555, 3)
	tmdbID, season, ok := domain.ParseSeriesID(original)
	require.True(t, ok)
	require.Equal(t, 555, tmdbID)
	require.Equal(t, 3, season)
	require.Equal(t, original, domain.SeriesStremioID(tmdbID, season))
}

func TestRoundTripEpisodeID(t *testing.T) {
	// Episode ID round trip: generate and parse
	original := domain.EpisodeStremioID(777, 2, 8)
	tmdbID, season, episode, ok := domain.ParseEpisodeID(original)
	require.True(t, ok)
	require.Equal(t, 777, tmdbID)
	require.Equal(t, 2, season)
	require.Equal(t, 8, episode)
	require.Equal(t, original, domain.EpisodeStremioID(tmdbID, season, episode))
}
