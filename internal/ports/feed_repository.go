package ports

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

type FeedRepository interface {
	ListEnabled(ctx context.Context) ([]domain.Feed, error)
	ListAll(ctx context.Context) ([]domain.Feed, error)
	Upsert(ctx context.Context, feed domain.Feed) (domain.Feed, error)
	Delete(ctx context.Context, id string) error
}
