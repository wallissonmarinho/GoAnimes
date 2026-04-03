package services

import (
	"context"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// TMDBBackdropCandidatesForEnrichment loads TMDB backdrop URLs (with WxH) via IMDb id when present, else TV search.
func TMDBBackdropCandidatesForEnrichment(ctx context.Context, tc *tmdb.Client, en domain.AniListSeriesEnrichment, searchTitle string) ([]domain.BackgroundCandidate, error) {
	if tc == nil {
		return nil, nil
	}
	if imdb := domain.NormalizeIMDbID(en.ImdbID); imdb != "" {
		return tc.BackdropCandidatesForIMDB(ctx, imdb)
	}
	q := strings.TrimSpace(en.TitlePreferred)
	if q == "" {
		q = strings.TrimSpace(searchTitle)
	}
	if q == "" {
		return nil, nil
	}
	return tc.BackdropCandidatesForTVSearch(ctx, q, en.StartYear)
}
