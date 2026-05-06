package admin

import (
	"context"
	"errors"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Service struct {
	Feeds   ports.FeedRepository
	Mapping ports.MappingRepository
	Catalog ports.CatalogRepository
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

func (s *Service) DeleteOverride(ctx context.Context, id string) error {
	ctx, span := tracer.Start(ctx, "admin.delete_override")
	span.SetAttributes(attribute.String("override.id", id))
	defer span.End()
	return s.Mapping.DeleteOverride(ctx, id)
}

func (s *Service) DeleteUnmatched(ctx context.Context, id string) error {
	ctx, span := tracer.Start(ctx, "admin.delete_unmatched")
	span.SetAttributes(attribute.String("unmatched.id", id))
	defer span.End()
	return s.Mapping.DeleteUnmatched(ctx, id)
}

func (s *Service) RemoveSourcesByProvider(ctx context.Context, provider string) (int, error) {
	ctx, span := tracer.Start(ctx, "admin.remove_sources_by_provider")
	span.SetAttributes(attribute.String("provider", provider))
	defer span.End()
	if s.Catalog == nil {
		return 0, context.Canceled
	}
	return s.Catalog.RemoveSourcesByProvider(ctx, provider)
}

func (s *Service) CleanFeedSources(ctx context.Context, feedID string) (removed int, feedName string, err error) {
	ctx, span := tracer.Start(ctx, "admin.clean_feed_sources")
	span.SetAttributes(attribute.String("feed.id", feedID))
	defer span.End()

	if feedID == "" {
		return 0, "", errors.New("feed ID required")
	}
	if s.Feeds == nil || s.Catalog == nil {
		return 0, "", errors.New("services not configured")
	}

	// Busca o feed pelo ID
	feeds, err := s.Feeds.ListAll(ctx)
	if err != nil {
		return 0, "", err
	}

	var feed *domain.Feed
	for i := range feeds {
		if feeds[i].ID == feedID {
			feed = &feeds[i]
			break
		}
	}

	if feed == nil {
		return 0, "", errors.New("feed not found")
	}

	// Remove as fontes do provider deste feed
	removed, err = s.Catalog.RemoveSourcesByProvider(ctx, feed.Name)
	if err != nil {
		return 0, "", err
	}

	return removed, feed.Name, nil
}
