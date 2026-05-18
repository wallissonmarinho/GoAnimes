package sync

import "time"

type Result struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Processed  int
	Errors     []error
}

type NormalizedRelease struct {
	RSSNameKey string
	Title      string
	Season     int
	Episode    int
	BatchStart int
	BatchEnd   int
	Quality    string
	MagnetLink string
	Provider   string
	Published  time.Time
}
