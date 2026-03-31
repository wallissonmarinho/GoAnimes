package domain

import "time"

// RSSSource is a persisted RSS feed URL.
type RSSSource struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}
