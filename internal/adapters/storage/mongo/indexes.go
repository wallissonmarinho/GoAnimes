package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	mongodb "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ensureIndexes(ctx context.Context, store *Store) error {
	if store == nil {
		return nil
	}
	_, err := store.Animes.Indexes().CreateMany(ctx, []mongodb.IndexModel{
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "tmdb_id", Value: 1}, bson.E{Key: "season_number", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "genres", Value: 1}}},
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "updated_at", Value: -1}}},
	})
	if err != nil {
		return err
	}
	_, err = store.Feeds.Indexes().CreateMany(ctx, []mongodb.IndexModel{
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "enabled", Value: 1}}},
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "updated_at", Value: -1}}},
	})
	if err != nil {
		return err
	}
	_, err = store.Overrides.Indexes().CreateMany(ctx, []mongodb.IndexModel{
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "rss_name_key", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "updated_at", Value: -1}}},
	})
	if err != nil {
		return err
	}
	_, err = store.Unmatched.Indexes().CreateMany(ctx, []mongodb.IndexModel{
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "rss_name_key", Value: 1}}},
		mongodb.IndexModel{Keys: bson.D{bson.E{Key: "last_seen_at", Value: -1}}},
	})
	return err
}
