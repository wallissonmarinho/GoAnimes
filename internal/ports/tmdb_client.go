package ports

import "context"

type TMDBSearchResult struct {
	TMDBID int
	Title  string
}

type TMDBSeasonDetails struct {
	Title        string
	Overview     string
	PosterPath   string
	BackdropPath string
	Genres       []string
	Rating       float64
}

type TMDBClient interface {
	SearchSeries(ctx context.Context, query string) (TMDBSearchResult, bool, error)
	GetSeasonDetails(ctx context.Context, tmdbID, season int) (TMDBSeasonDetails, error)
}
