package ports

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

type MappingRepository interface {
	FindOverride(ctx context.Context, rssNameKey string) (domain.MappingOverride, bool, error)
	UpsertOverride(ctx context.Context, override domain.MappingOverride) (domain.MappingOverride, error)
	ListOverrides(ctx context.Context) ([]domain.MappingOverride, error)
	AddUnmatched(ctx context.Context, release domain.UnmatchedRelease) error
	ListUnmatched(ctx context.Context, limit int) ([]domain.UnmatchedRelease, error)
}
