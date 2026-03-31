package anilist_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
)

func TestClient_SearchAnimePoster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Page": {
					"media": [{
						"title": {"userPreferred": "Test Anime", "romaji": "Test Anime", "english": "Test Anime"},
						"coverImage": {"extraLarge": "https://cdn.example/poster.jpg", "large": ""}
					}]
				}
			}
		}`))
	}))
	defer srv.Close()

	g := httpclient.NewGetter(5*time.Second, "GoAnimes/test", 1<<20)
	c := anilist.NewClient(g, anilist.WithEndpoint(srv.URL))

	res, err := c.SearchAnimePoster(context.Background(), "Test Anime")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/poster.jpg", res.PosterURL)
	require.Equal(t, "Test Anime", res.Title)
}
