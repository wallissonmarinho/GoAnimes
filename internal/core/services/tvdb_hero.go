package services

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/thetvdb"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// TVDBBackdropCandidatesForEnrichment loads TheTVDB fanart/backdrop URLs when TvdbSeriesID or IMDb id is known.
func TVDBBackdropCandidatesForEnrichment(ctx context.Context, tc *thetvdb.Client, en domain.SeriesEnrichment) ([]domain.BackgroundCandidate, error) {
	if tc == nil {
		return nil, nil
	}
	sid := en.TvdbSeriesID
	var err error
	if sid <= 0 {
		imdb := domain.NormalizeIMDbID(en.ImdbID)
		if imdb == "" {
			return nil, nil
		}
		sid, err = tc.SeriesIDByIMDbRemote(ctx, imdb)
		if err != nil || sid <= 0 {
			return nil, err
		}
	}
	return tc.SeriesFanartCandidates(ctx, sid)
}
