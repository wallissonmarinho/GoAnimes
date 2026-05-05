package ports

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

type CatalogRepository interface {
	UpsertSeason(ctx context.Context, anime domain.Anime) error
	AddEpisodeSource(ctx context.Context, tmdbID, season, episode int, src domain.Source) (bool, error)
	UpdateEpisodeDetails(ctx context.Context, tmdbID, season, episode int, title, overview, stillPath string) error
	GetByTMDBSeason(ctx context.Context, tmdbID, season int) (domain.Anime, bool, error)
	ListByGenre(ctx context.Context, genre string, limit, skip int) ([]domain.Anime, error)
	ListRecent(ctx context.Context, days, limit, skip int) ([]domain.Anime, error)
	ListAll(ctx context.Context, limit, skip int) ([]domain.Anime, error)
	ListGenres(ctx context.Context) ([]string, error)
	RemoveSourcesByProvider(ctx context.Context, provider string) (removedCount int, err error)
}
