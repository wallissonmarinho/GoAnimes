package ports

import (
	"context"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

type ReleaseItem struct {
	Title     string
	Magnet    string
	Link      string
	Provider  string
	Quality   string
	Published time.Time
}

type FeedReader interface {
	Fetch(ctx context.Context, feed domain.Feed) ([]ReleaseItem, error)
}
