package mongo

import "time"

type animeDoc struct {
	ID            string       `bson:"_id,omitempty"`
	TMDBID        int          `bson:"tmdb_id"`
	SeasonNumber  int          `bson:"season_number"`
	Title         string       `bson:"title"`
	Overview      string       `bson:"overview"`
	Genres        []string     `bson:"genres"`
	Rating        float64      `bson:"rating"`
	PosterPath    string       `bson:"poster_path"`
	BackdropPath  string       `bson:"backdrop_path"`
	Episodes      []episodeDoc `bson:"episodes"`
	MappingStatus string       `bson:"mapping_status"`
	UpdatedAt     time.Time    `bson:"updated_at"`
}

type episodeDoc struct {
	Number    int         `bson:"number"`
	Title     string      `bson:"title"`
	Overview  string      `bson:"overview"`
	StillPath string      `bson:"still_path"`
	Sources   []sourceDoc `bson:"sources"`
	AddedAt   time.Time   `bson:"added_at"`
}

type sourceDoc struct {
	Provider   string `bson:"provider"`
	MagnetLink string `bson:"magnet_link"`
	Quality    string `bson:"quality"`
}

type feedDoc struct {
	ID        string    `bson:"_id,omitempty"`
	Name      string    `bson:"name"`
	URL       string    `bson:"url"`
	Type      string    `bson:"type"`
	Enabled   bool      `bson:"enabled"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type mappingOverrideDoc struct {
	ID         string    `bson:"_id,omitempty"`
	RSSNameKey string    `bson:"rss_name_key"`
	TMDBID     int       `bson:"tmdb_id"`
	Season     int       `bson:"season"`
	Locked     bool      `bson:"locked"`
	UpdatedAt  time.Time `bson:"updated_at"`
}

type unmatchedDoc struct {
	ID         string    `bson:"_id,omitempty"`
	RSSNameKey string    `bson:"rss_name_key"`
	RawTitle   string    `bson:"raw_title"`
	Provider   string    `bson:"provider"`
	AddedAt    time.Time `bson:"added_at"`
	LastSeenAt time.Time `bson:"last_seen_at"`
	Count      int       `bson:"count"`
}
