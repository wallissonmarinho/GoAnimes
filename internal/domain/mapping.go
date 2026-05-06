package domain

import "time"

type FeedType string

const (
	FeedTypeRSS     FeedType = "rss"
	FeedTypeTorznab FeedType = "torznab"
)

type Feed struct {
	ID        string
	Name      string
	URL       string
	Type      FeedType
	Enabled   bool
	UpdatedAt time.Time
}

type MappingOverride struct {
	ID            string
	RSSNameKey    string
	TMDBID        int
	Season        int
	Locked        bool
	EpisodeOffset int
	UpdatedAt     time.Time
}

type UnmatchedRelease struct {
	ID         string
	RSSNameKey string
	RawTitle   string
	Provider   string
	AddedAt    time.Time
	LastSeenAt time.Time
	Count      int
}
