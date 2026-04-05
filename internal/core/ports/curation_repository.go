package ports

import (
	"context"
	"time"
)

// AI curation statuses for series_curation.ai_review_status (Phase D — endpoint & adapter later).
const (
	SeriesCurationPending    = "pending"
	SeriesCurationOK         = "ok"
	SeriesCurationNeedsHuman = "needs_human"
)

// SeriesCuration is persisted AI review metadata per catalog series (see migrations 00004).
type SeriesCuration struct {
	SeriesID                 string
	LastAIReviewAt           time.Time
	AIReviewStatus           string
	CanonicalTitleSuggestion string
	SeasonCountSuggestion    *int
	RawAIPayloadJSON         string
}

// CurationRepository will back the future AI curation use case (list pending, apply suggestions in a transaction).
type CurationRepository interface {
	GetSeriesCuration(ctx context.Context, seriesID string) (*SeriesCuration, error)
}
