package domain

import "time"

// CatalogItem is one Stremio catalog entry (movie) backed by a torrent/magnet link.
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
}

// CatalogSnapshot is persisted merge output for hydration after restart.
type CatalogSnapshot struct {
	OK         bool
	Message    string
	ItemCount  int
	StartedAt  time.Time
	FinishedAt time.Time
	Items      []CatalogItem
}

// SyncResult is the outcome of an RSS sync job.
type SyncResult struct {
	Message string
	Errors  []string
}
