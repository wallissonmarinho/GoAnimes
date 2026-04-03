package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestNormalizeIMDbID(t *testing.T) {
	require.Equal(t, "tt1234567", domain.NormalizeIMDbID("tt1234567"))
	require.Equal(t, "tt1234567", domain.NormalizeIMDbID("https://www.imdb.com/title/tt1234567/"))
	require.Equal(t, "", domain.NormalizeIMDbID(""))
}

func TestPickBestStremioBackground_prefersNear1280x720(t *testing.T) {
	cands := []domain.BackgroundCandidate{
		{URL: "https://portrait.example/p.jpg", W: 500, H: 750},
		{URL: "https://wide.example/b.jpg", W: 1280, H: 720},
		{URL: "https://ultrawide.example/u.jpg", W: 3840, H: 1080},
	}
	got := domain.PickBestStremioBackground(cands, "https://poster.example/fallback.jpg")
	require.Equal(t, "https://wide.example/b.jpg", got)
}

func TestResolveStremioHeroBackground_mergesTMDB(t *testing.T) {
	en := domain.AniListSeriesEnrichment{
		PosterURL:        "https://p.example/p.jpg",
		AniListBannerURL: "https://al.example/banner.jpg",
	}
	tmdb := []domain.BackgroundCandidate{{URL: "https://tmdb.example/bg.jpg", W: 1280, H: 720}}
	got := domain.ResolveStremioHeroBackground(en, tmdb)
	require.Equal(t, "https://tmdb.example/bg.jpg", got)
}
