package stremio

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestMetaUsesTheTVDBData(t *testing.T) {
	require.False(t, MetaUsesTheTVDBData(domain.SeriesEnrichment{}))
	require.True(t, MetaUsesTheTVDBData(domain.SeriesEnrichment{TvdbSeriesID: 1}))
	require.True(t, MetaUsesTheTVDBData(domain.SeriesEnrichment{
		StremioHeroBackgroundURL: "https://artworks.thetvdb.com/banners/fanart/x.jpg",
	}))
	require.True(t, MetaUsesTheTVDBData(domain.SeriesEnrichment{
		EpisodeThumbnailByNum: map[int]string{1: "https://artworks.thetvdb.com/ep/1.jpg"},
	}))
}

func TestAppendTheTVDBAttributionToDescription(t *testing.T) {
	require.Contains(t, AppendTheTVDBAttributionToDescription(""), "thetvdb.com")
	require.Contains(t, AppendTheTVDBAttributionToDescription("Synopsis."), "Synopsis.")
	require.Contains(t, AppendTheTVDBAttributionToDescription("Synopsis."), "thetvdb.com")
}
