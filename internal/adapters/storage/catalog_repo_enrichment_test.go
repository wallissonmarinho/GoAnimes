package storage

import (
	"testing"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestEnrichmentEpisodeMapsRoundTrip(t *testing.T) {
	en := domain.SeriesEnrichment{
		SeriesStatus:      "Continuing",
		SeriesReleasedISO: "2019-10-02T00:00:00.000Z",
		SeriesYearLabel:   "2019–2022",
		EpisodeTitleByNum: map[int]string{
			1:  "First",
			12: "Twelve",
		},
		EpisodeThumbnailByNum: map[int]string{
			1: "https://a.example/t1.jpg",
		},
		EpisodeReleasedBySeasonEpisode: map[string]string{
			"1:5": "2026-05-01T12:00:00.000Z",
		},
	}
	raw, err := enrichmentEpisodeMapsJSON(en)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.SeriesEnrichment
	if err := enrichmentFromEpisodeMapsJSON(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.EpisodeTitleByNum[1] != "First" || got.EpisodeTitleByNum[12] != "Twelve" {
		t.Fatalf("titles: %#v", got.EpisodeTitleByNum)
	}
	if got.EpisodeThumbnailByNum[1] != "https://a.example/t1.jpg" {
		t.Fatalf("thumbs: %#v", got.EpisodeThumbnailByNum)
	}
	if got.EpisodeReleasedBySeasonEpisode["1:5"] != "2026-05-01T12:00:00.000Z" {
		t.Fatalf("released se: %#v", got.EpisodeReleasedBySeasonEpisode)
	}
	if got.SeriesStatus != "Continuing" || got.SeriesReleasedISO != "2019-10-02T00:00:00.000Z" || got.SeriesYearLabel != "2019–2022" {
		t.Fatalf("series cinemeta fields: %#v", got)
	}
}
