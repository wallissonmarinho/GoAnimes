package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestFilterSeriesWithRecentReleases(t *testing.T) {
	sid := domain.SeriesStremioID("Week Show")
	today := time.Now().UTC().Format("2006-01-02")
	old := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	snap := &domain.CatalogSnapshot{
		Series: []domain.CatalogSeries{{ID: sid, Name: "Week Show"}},
		Items: []domain.CatalogItem{
			{SeriesID: sid, Name: "[Torrent] Week Show - 01 [720p][br]", Released: old},
			{SeriesID: sid, Name: "[Torrent] Week Show - 02 [720p][br]", Released: today},
		},
	}
	out := domain.FilterSeriesWithRecentReleases(snap, 7)
	require.Len(t, out, 1)
	require.Equal(t, sid, out[0].ID)
}

func TestFilterSeriesByGenre(t *testing.T) {
	series := []domain.CatalogSeries{
		{ID: "a", Name: "A", Genres: []string{"Comédia", "Fantasia"}},
		{ID: "b", Name: "B", Genres: []string{"Ação"}},
	}
	out := domain.FilterSeriesByGenre(series, "fantasia")
	require.Len(t, out, 1)
	require.Equal(t, "a", out[0].ID)
}

func TestFilterSeriesWithRecentReleases_onlyOld(t *testing.T) {
	sid := domain.SeriesStremioID("Stale Show")
	old := time.Now().UTC().AddDate(0, 0, -20).Format("2006-01-02")
	snap := &domain.CatalogSnapshot{
		Series: []domain.CatalogSeries{{ID: sid, Name: "Stale Show"}},
		Items: []domain.CatalogItem{
			{SeriesID: sid, Name: "[Torrent] Stale Show - 01 [720p][br]", Released: old},
		},
	}
	require.Nil(t, domain.FilterSeriesWithRecentReleases(snap, 7))
}
