package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestParseItemReleasedDate_rfc3339PreservesWallClock(t *testing.T) {
	ts, ok := domain.ParseItemReleasedDate("2026-04-04T16:01:13Z")
	require.True(t, ok)
	require.Equal(t, 16, ts.UTC().Hour())
	require.Equal(t, 1, ts.UTC().Minute())
}

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

	en := []domain.CatalogSeries{{ID: "c", Name: "C", Genres: []string{"Action", "Comedy"}}}
	require.Len(t, domain.FilterSeriesByGenre(en, "Ação"), 1)
	require.Len(t, domain.FilterSeriesByGenre(en, "Comédia"), 1)
}

func TestUniqueGenreLabelsFromCatalogSeries(t *testing.T) {
	require.Empty(t, domain.UniqueGenreLabelsFromCatalogSeries(nil))
	require.Empty(t, domain.UniqueGenreLabelsFromCatalogSeries([]domain.CatalogSeries{{ID: "x", Genres: nil}}))
	got := domain.UniqueGenreLabelsFromCatalogSeries([]domain.CatalogSeries{
		{ID: "a", Genres: []string{"Comédia", " Ação "}},
		{ID: "b", Genres: []string{"Ação", "Fantasia"}},
	})
	require.Equal(t, []string{"Ação", "Comédia", "Fantasia"}, got)

	mergeEN := domain.UniqueGenreLabelsFromCatalogSeries([]domain.CatalogSeries{
		{ID: "x", Genres: []string{"Action", "Comedy"}},
		{ID: "y", Genres: []string{"Ação"}},
	})
	require.Equal(t, []string{"Ação", "Comédia"}, mergeEN)
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
