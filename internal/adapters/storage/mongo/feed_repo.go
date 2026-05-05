package mongo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeedRepository struct {
	store *Store
}

func NewFeedRepository(store *Store) *FeedRepository {
	return &FeedRepository{store: store}
}

func (r *FeedRepository) ListEnabled(ctx context.Context) ([]domain.Feed, error) {
	return r.list(ctx, bson.M{"enabled": true})
}

func (r *FeedRepository) ListAll(ctx context.Context) ([]domain.Feed, error) {
	return r.list(ctx, bson.M{})
}

func (r *FeedRepository) Upsert(ctx context.Context, feed domain.Feed) (domain.Feed, error) {
	if feed.ID == "" {
		feed.ID = uuid.NewString()
	}
	feed.UpdatedAt = time.Now().UTC()
	doc := feedDoc{
		ID: feed.ID, Name: feed.Name, URL: feed.URL, Type: string(feed.Type), Enabled: feed.Enabled, UpdatedAt: feed.UpdatedAt,
	}
	filter := bson.M{"_id": feed.ID}
	update := bson.M{
		"$set": bson.M{
			"name":       doc.Name,
			"url":        doc.URL,
			"type":       doc.Type,
			"enabled":    doc.Enabled,
			"updated_at": doc.UpdatedAt,
		},
		"$setOnInsert": bson.M{"_id": doc.ID},
	}
	_, err := r.store.Feeds.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return feed, err
}

func (r *FeedRepository) Delete(ctx context.Context, id string) error {
	_, err := r.store.Feeds.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *FeedRepository) list(ctx context.Context, filter bson.M) ([]domain.Feed, error) {
	cur, err := r.store.Feeds.Find(ctx, filter, options.Find().SetSort(bson.D{bson.E{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Feed{}
	for cur.Next(ctx) {
		var doc feedDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, domain.Feed{
			ID: doc.ID, Name: doc.Name, URL: doc.URL, Type: domain.FeedType(doc.Type), Enabled: doc.Enabled, UpdatedAt: doc.UpdatedAt,
		})
	}
	return out, cur.Err()
}
