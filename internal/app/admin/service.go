package admin

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Service struct {
	Feeds   ports.FeedRepository
	Mapping ports.MappingRepository
}

var tracer = otel.Tracer("goanimes/admin")

func (s *Service) ListFeeds(ctx context.Context) ([]domain.Feed, error) {
	ctx, span := tracer.Start(ctx, "admin.list_feeds")
	defer span.End()
	return s.Feeds.ListAll(ctx)
}

func (s *Service) UpsertFeed(ctx context.Context, feed domain.Feed) (domain.Feed, error) {
	ctx, span := tracer.Start(ctx, "admin.upsert_feed")
	span.SetAttributes(attribute.String("feed.id", feed.ID))
	defer span.End()
	return s.Feeds.Upsert(ctx, feed)
}

func (s *Service) DeleteFeed(ctx context.Context, id string) error {
	ctx, span := tracer.Start(ctx, "admin.delete_feed")
	span.SetAttributes(attribute.String("feed.id", id))
	defer span.End()
	return s.Feeds.Delete(ctx, id)
}

func (s *Service) ListOverrides(ctx context.Context) ([]domain.MappingOverride, error) {
	ctx, span := tracer.Start(ctx, "admin.list_overrides")
	defer span.End()
	return s.Mapping.ListOverrides(ctx)
}

func (s *Service) UpsertOverride(ctx context.Context, override domain.MappingOverride) (domain.MappingOverride, error) {
	ctx, span := tracer.Start(ctx, "admin.upsert_override")
	span.SetAttributes(
		attribute.String("override.id", override.ID),
		attribute.String("override.rss_name_key", override.RSSNameKey),
	)
	defer span.End()
	return s.Mapping.UpsertOverride(ctx, override)
}

func (s *Service) ListUnmatched(ctx context.Context, limit int) ([]domain.UnmatchedRelease, error) {
	ctx, span := tracer.Start(ctx, "admin.list_unmatched")
	span.SetAttributes(attribute.Int("unmatched.limit", limit))
	defer span.End()
	return s.Mapping.ListUnmatched(ctx, limit)
}
