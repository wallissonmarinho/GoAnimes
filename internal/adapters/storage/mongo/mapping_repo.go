package mongo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	mongodb "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MappingRepository struct {
	store *Store
}

func NewMappingRepository(store *Store) *MappingRepository {
	return &MappingRepository{store: store}
}

func (r *MappingRepository) FindOverride(ctx context.Context, rssNameKey string) (domain.MappingOverride, bool, error) {
	var doc mappingOverrideDoc
	err := r.store.Overrides.FindOne(ctx, bson.M{"rss_name_key": rssNameKey}).Decode(&doc)
	if err != nil {
		if err == mongodb.ErrNoDocuments {
			return domain.MappingOverride{}, false, nil
		}
		return domain.MappingOverride{}, false, err
	}
	return domain.MappingOverride{
		ID: doc.ID, RSSNameKey: doc.RSSNameKey, TMDBID: doc.TMDBID, Season: doc.Season, Locked: doc.Locked, UpdatedAt: doc.UpdatedAt,
	}, true, nil
}

func (r *MappingRepository) UpsertOverride(ctx context.Context, override domain.MappingOverride) (domain.MappingOverride, error) {
	if override.ID == "" {
		override.ID = uuid.NewString()
	}
	override.UpdatedAt = time.Now().UTC()
	doc := mappingOverrideDoc{
		ID: override.ID, RSSNameKey: override.RSSNameKey, TMDBID: override.TMDBID, Season: override.Season, Locked: override.Locked, UpdatedAt: override.UpdatedAt,
	}
	filter := bson.M{"rss_name_key": override.RSSNameKey}
	update := bson.M{
		"$set": bson.M{
			"rss_name_key": doc.RSSNameKey,
			"tmdb_id":      doc.TMDBID,
			"season":       doc.Season,
			"locked":       doc.Locked,
			"updated_at":   doc.UpdatedAt,
		},
		"$setOnInsert": bson.M{"_id": doc.ID},
	}
	_, err := r.store.Overrides.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return override, err
}

func (r *MappingRepository) ListOverrides(ctx context.Context) ([]domain.MappingOverride, error) {
	cur, err := r.store.Overrides.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{bson.E{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.MappingOverride{}
	for cur.Next(ctx) {
		var doc mappingOverrideDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, domain.MappingOverride{
			ID: doc.ID, RSSNameKey: doc.RSSNameKey, TMDBID: doc.TMDBID, Season: doc.Season, Locked: doc.Locked, UpdatedAt: doc.UpdatedAt,
		})
	}
	return out, cur.Err()
}

func (r *MappingRepository) AddUnmatched(ctx context.Context, release domain.UnmatchedRelease) error {
	if release.ID == "" {
		release.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if release.AddedAt.IsZero() {
		release.AddedAt = now
	}
	release.LastSeenAt = now
	release.Count++
	doc := unmatchedDoc{
		ID: release.ID, RSSNameKey: release.RSSNameKey, RawTitle: release.RawTitle, Provider: release.Provider, AddedAt: release.AddedAt, LastSeenAt: release.LastSeenAt, Count: release.Count,
	}
	filter := bson.M{"rss_name_key": release.RSSNameKey}
	update := bson.M{
		"$set": bson.M{
			"raw_title":    doc.RawTitle,
			"provider":     doc.Provider,
			"last_seen_at": doc.LastSeenAt,
			"count":        doc.Count,
			"rss_name_key": doc.RSSNameKey,
		},
		"$setOnInsert": bson.M{"_id": doc.ID, "added_at": doc.AddedAt},
	}
	_, err := r.store.Unmatched.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *MappingRepository) ListUnmatched(ctx context.Context, limit int) ([]domain.UnmatchedRelease, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	cur, err := r.store.Unmatched.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{bson.E{Key: "last_seen_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.UnmatchedRelease{}
	for cur.Next(ctx) {
		var doc unmatchedDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, domain.UnmatchedRelease{
			ID: doc.ID, RSSNameKey: doc.RSSNameKey, RawTitle: doc.RawTitle, Provider: doc.Provider, AddedAt: doc.AddedAt, LastSeenAt: doc.LastSeenAt, Count: doc.Count,
		})
	}
	return out, cur.Err()
}
