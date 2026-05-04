package mongo

import (
	"context"
	"time"

	mongodb "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Store struct {
	Client    *mongodb.Client
	Database  *mongodb.Database
	Animes    *mongodb.Collection
	Feeds     *mongodb.Collection
	Overrides *mongodb.Collection
	Unmatched *mongodb.Collection
}

func Connect(ctx context.Context, uri, dbName string) (*Store, error) {
	client, err := mongodb.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	db := client.Database(dbName)
	store := &Store{
		Client:    client,
		Database:  db,
		Animes:    db.Collection("animes"),
		Feeds:     db.Collection("feeds"),
		Overrides: db.Collection("mapping_overrides"),
		Unmatched: db.Collection("unmatched_releases"),
	}
	if err := ensureIndexes(ctx, store); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return store, nil
}

func Disconnect(ctx context.Context, store *Store) error {
	if store == nil || store.Client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return store.Client.Disconnect(ctx)
}
