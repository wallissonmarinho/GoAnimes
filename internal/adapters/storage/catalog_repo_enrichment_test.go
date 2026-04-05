package storage

import (
	"testing"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestEnrichmentEpisodeMapsRoundTrip(t *testing.T) {
	en := domain.AniListSeriesEnrichment{
		EpisodeTitleByNum: map[int]string{
			1: "First",
			12: "Twelve",
		},
		EpisodeThumbnailByNum: map[int]string{
			1: "https://a.example/t1.jpg",
		},
	}
	raw, err := enrichmentEpisodeMapsJSON(en)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.AniListSeriesEnrichment
	if err := enrichmentFromEpisodeMapsJSON(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.EpisodeTitleByNum[1] != "First" || got.EpisodeTitleByNum[12] != "Twelve" {
		t.Fatalf("titles: %#v", got.EpisodeTitleByNum)
	}
	if got.EpisodeThumbnailByNum[1] != "https://a.example/t1.jpg" {
		t.Fatalf("thumbs: %#v", got.EpisodeThumbnailByNum)
	}
}
