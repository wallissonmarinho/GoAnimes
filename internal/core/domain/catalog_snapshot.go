package domain

import "time"

// CatalogSeries is one anime show in Discover (Stremio catalog + meta use type "anime").
type CatalogSeries struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Poster string `json:"poster,omitempty"`
}

// CatalogItem is one release (episode). Streams use its ID; catalog lists the parent series.
type CatalogItem struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	Poster       string `json:"poster,omitempty"`
	MagnetURL    string `json:"magnet_url,omitempty"`
	TorrentURL   string `json:"torrent_url,omitempty"`
	InfoHash     string `json:"info_hash,omitempty"`
	Released     string `json:"released,omitempty"`
	SubtitlesTag string `json:"subtitles_tag,omitempty"`
	SeriesID     string `json:"series_id,omitempty"`
	SeriesName   string `json:"series_name,omitempty"`
	Season       int    `json:"season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	IsSpecial    bool   `json:"is_special,omitempty"`
}

// CatalogSnapshot is persisted merge output for hydration after restart.
type CatalogSnapshot struct {
	OK             bool
	Message        string
	ItemCount      int
	StartedAt      time.Time
	FinishedAt     time.Time
	Items          []CatalogItem
	Series          []CatalogSeries                        `json:"-"`
	AniListBySeries map[string]AniListSeriesEnrichment `json:"-"` // persisted as anilist_series (+ legacy anilist_posters)
}

// SyncResult is the outcome of an RSS sync job.
type SyncResult struct {
	Message string
	Errors  []string
}
