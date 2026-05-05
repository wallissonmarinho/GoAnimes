package integration
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func mongoStore(t *testing.T, ctx context.Context) *mongo.Client {
	opts := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(ctx, opts)
	require.NoError(t, err)
	require.NoError(t, client.Ping(ctx, nil))
	return client
}

// TestEpisodeMetadataFlow verifies that episode details are properly updated and retrieved
func TestEpisodeMetadataFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup MongoDB connection
	client := mongoStore(t, ctx)
	defer func() {
		_ = client.Database("goanimes_test").Drop(ctx)
		_ = client.Disconnect(ctx)
	}()

	db := client.Database("goanimes_test")
	store := &mongoStore{db: db}
	repo := newCatalogRepository(store)

	// Step 1: Create episode with source (like AddEpisodeSource does)
	_, err := repo.AddEpisodeSource(ctx, 12345, 1, 1, domain.Source{
		Provider:   "TestProvider",
		MagnetLink: "magnet:?xt=urn:btih:test",
		Quality:    "1080p",
	})
	require.NoError(t, err)

	// Verify episode was created
	anime1, found, err := repo.GetByTMDBSeason(ctx, 12345, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, anime1.Episodes, 1)
	require.Equal(t, 1, anime1.Episodes[0].Number)
	require.Empty(t, anime1.Episodes[0].Title)
	require.Empty(t, anime1.Episodes[0].Overview)
	require.Empty(t, anime1.Episodes[0].StillPath)

	// Step 2: Update episode with TMDB details (like enrichEpisodeDetails does)
	err = repo.UpdateEpisodeDetails(ctx, 12345, 1, 1, "Episode Title", "Episode Overview", "https://example.com/image.jpg")
	require.NoError(t, err)

	// Step 3: Retrieve and verify details were saved
	anime2, found, err := repo.GetByTMDBSeason(ctx, 12345, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, anime2.Episodes, 1)
	ep := anime2.Episodes[0]
	require.Equal(t, "Episode Title", ep.Title)
	require.Equal(t, "Episode Overview", ep.Overview)
	require.Equal(t, "https://example.com/image.jpg", ep.StillPath)

	// Step 4: Verify UpsertSeason doesn't lose details
	anime2.Title = "Updated Anime Title"
	err = repo.UpsertSeason(ctx, anime2)
	require.NoError(t, err)

	// Step 5: Verify all details are still there
	anime3, found, err := repo.GetByTMDBSeason(ctx, 12345, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Updated Anime Title", anime3.Title)
	require.Len(t, anime3.Episodes, 1)
	ep = anime3.Episodes[0]
	require.Equal(t, "Episode Title", ep.Title)
	require.Equal(t, "Episode Overview", ep.Overview)
	require.Equal(t, "https://example.com/image.jpg", ep.StillPath)
}
