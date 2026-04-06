package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestMergeSnapshotSeriesBySharedMalID_noop_singleMalPerSeries(t *testing.T) {
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "a:1", SeriesID: "goanimes:series:aaaaaaaa", SeriesName: "X", Season: 1, Episode: 1, Name: "x1"},
			{ID: "a:2", SeriesID: "goanimes:series:bbbbbbbb", SeriesName: "Y", Season: 1, Episode: 1, Name: "y1"},
		},
		AniListBySeries: map[string]domain.AniListSeriesEnrichment{
			"goanimes:series:aaaaaaaa": {MalID: 1, TitlePreferred: "A"},
			"goanimes:series:bbbbbbbb": {MalID: 2, TitlePreferred: "B"},
		},
	}
	snap.Series = domain.BuildSeriesList(snap.Items)
	domain.MergeSnapshotSeriesBySharedMalID(snap)
	require.Len(t, snap.Series, 2)
	require.Equal(t, "goanimes:series:aaaaaaaa", snap.Items[0].SeriesID)
	require.Equal(t, "goanimes:series:bbbbbbbb", snap.Items[1].SeriesID)
}

func TestMergeSnapshotSeriesBySharedMalID_mergesSameMalID(t *testing.T) {
	idA := "goanimes:series:aaaaaaaa"
	idB := "goanimes:series:bbbbbbbb"
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "x:1", SeriesID: idA, SeriesName: "Show", Season: 1, Episode: 1, Name: "e1"},
			{ID: "x:2", SeriesID: idA, SeriesName: "Show", Season: 1, Episode: 2, Name: "e2"},
			{ID: "x:3", SeriesID: idB, SeriesName: "Show", Season: 1, Episode: 3, Name: "e3"},
		},
		AniListBySeries: map[string]domain.AniListSeriesEnrichment{
			idA: {MalID: 99, TitlePreferred: "United", Description: "from A"},
			idB: {MalID: 99, TitlePreferred: "United", PosterURL: "https://p"},
		},
	}
	snap.Series = domain.BuildSeriesList(snap.Items)
	domain.MergeSnapshotSeriesBySharedMalID(snap)

	for _, it := range snap.Items {
		require.Equal(t, idA, it.SeriesID, "canonical should be idA (more items)")
	}
	require.Len(t, snap.Series, 1)
	require.Equal(t, idA, snap.Series[0].ID)

	en := snap.AniListBySeries[idA]
	require.Equal(t, 99, en.MalID)
	require.Contains(t, en.Description, "from A")
	require.Contains(t, en.PosterURL, "https://p")
	_, hasB := snap.AniListBySeries[idB]
	require.False(t, hasB)
}

func TestMergeSnapshotSeriesBySharedMalID_tieBreakLexicographic(t *testing.T) {
	idLo := "goanimes:series:aaaaaaaa"
	idHi := "goanimes:series:zzzzzzzz"
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "x:1", SeriesID: idLo, SeriesName: "S", Season: 1, Episode: 1, Name: "e1"},
			{ID: "x:2", SeriesID: idHi, SeriesName: "S", Season: 1, Episode: 2, Name: "e2"},
		},
		AniListBySeries: map[string]domain.AniListSeriesEnrichment{
			idLo: {MalID: 5},
			idHi: {MalID: 5},
		},
	}
	snap.Series = domain.BuildSeriesList(snap.Items)
	domain.MergeSnapshotSeriesBySharedMalID(snap)
	for _, it := range snap.Items {
		require.Equal(t, idLo, it.SeriesID)
	}
}

func TestMergeSnapshotSeriesBySharedMalID_skipsZeroMalID(t *testing.T) {
	idA := "goanimes:series:aaaaaaaa"
	idB := "goanimes:series:bbbbbbbb"
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "x:1", SeriesID: idA, SeriesName: "S1", Season: 1, Episode: 1, Name: "e1"},
			{ID: "x:2", SeriesID: idB, SeriesName: "S2", Season: 1, Episode: 2, Name: "e2"},
		},
		AniListBySeries: map[string]domain.AniListSeriesEnrichment{
			idA: {MalID: 0, TitlePreferred: "A"},
			idB: {MalID: 0, TitlePreferred: "B"},
		},
	}
	snap.Series = domain.BuildSeriesList(snap.Items)
	domain.MergeSnapshotSeriesBySharedMalID(snap)
	require.Len(t, snap.Series, 2)
}
