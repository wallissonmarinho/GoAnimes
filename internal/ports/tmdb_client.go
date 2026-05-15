package ports

import "context"

type TMDBSearchResult struct {
	TMDBID int
	Title  string
}

type TMDBSeasonDetails struct {
	Title             string
	OriginalTitle     string
	Overview          string
	PosterPath        string
	BackdropPath      string
	LogoPath          string
	Genres            []string
	Rating            float64
	VoteCount         int
	Popularity        float64
	FirstAirDate      string
	LastAirDate       string
	LastEpisodeAirDate string
	LastEpisodeNumber int
	NextEpisodeAirDate string
	NextEpisodeNumber int
	Status            string
	InProduction      bool
	HasNextEpisode    bool
	TVType            string
	EpisodeRunTime    []int
	SeasonRunTime     []int
	SeasonVoteAverage float64
}

type TMDBEpisodeDetails struct {
	AirDate   string
	Title     string
	Overview  string
	StillPath string
}

type TMDBClient interface {
	SearchSeries(ctx context.Context, query string) (TMDBSearchResult, bool, error)
	GetSeasonDetails(ctx context.Context, tmdbID, season int) (TMDBSeasonDetails, error)
	GetEpisodeDetails(ctx context.Context, tmdbID, season, episode int) (TMDBEpisodeDetails, error)
}
