package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestMergeCatalogItemsByID_keepsOldAddsNewRefreshesSameID(t *testing.T) {
	prev := []domain.CatalogItem{
		{ID: "goanimes:old1", Name: "Old ep 1"},
		{ID: "goanimes:old2", Name: "Old ep 2"},
	}
	incoming := []domain.CatalogItem{
		{ID: "goanimes:old1", Name: "Refreshed ep 1"},
		{ID: "goanimes:new3", Name: "New ep 3"},
	}
	out := domain.MergeCatalogItemsByID(prev, incoming)
	require.Len(t, out, 3)
	by := make(map[string]string)
	for _, it := range out {
		by[it.ID] = it.Name
	}
	require.Equal(t, "Refreshed ep 1", by["goanimes:old1"])
	require.Equal(t, "Old ep 2", by["goanimes:old2"])
	require.Equal(t, "New ep 3", by["goanimes:new3"])
}

func TestSortCatalogItemsInPlace_seriesSeasonEpisode(t *testing.T) {
	items := []domain.CatalogItem{
		{ID: "b", SeriesID: "s2", Season: 1, Episode: 2, Released: "2025-01-01"},
		{ID: "a", SeriesID: "s1", Season: 1, Episode: 1, Released: "2025-01-02"},
		{ID: "c", SeriesID: "s1", Season: 1, Episode: 2, Released: "2025-01-03"},
	}
	domain.SortCatalogItemsInPlace(items)
	require.Equal(t, []string{"a", "c", "b"}, []string{items[0].ID, items[1].ID, items[2].ID})
}
