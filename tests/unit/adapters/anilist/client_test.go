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

func TestClient_SearchAnimeMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Page": {
					"media": [{
						"title": {"userPreferred": "Test Anime", "romaji": "Test Anime", "english": "Test Anime"},
						"coverImage": {"extraLarge": "https://cdn.example/poster.jpg", "large": ""},
						"bannerImage": {"large": "https://cdn.example/banner.jpg"},
						"description": "A <b>fine</b> show.",
						"genres": ["Action", "Comedy"],
						"seasonYear": 2024,
						"startDate": {"year": 2024},
						"duration": 24,
						"trailer": {"id": "abc123xyz", "site": "youtube"},
						"streamingEpisodes": [
							{"title": "Episode 1 - Pilot"},
							{"title": "Episode 2 - The Chase"}
						]
					}]
				}
			}
		}`))
	}))
	defer srv.Close()

	g := httpclient.NewGetter(5*time.Second, "GoAnimes/test", 1<<20)
	c := anilist.NewClient(g, anilist.WithEndpoint(srv.URL))

	res, err := c.SearchAnimeMedia(context.Background(), "Test Anime")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/poster.jpg", res.PosterURL)
	require.Equal(t, "https://cdn.example/banner.jpg", res.BackgroundURL)
	require.Equal(t, "Test Anime", res.Title)
	require.Contains(t, res.Description, "fine")
	require.Equal(t, []string{"Action", "Comedy"}, res.Genres)
	require.Equal(t, 2024, res.StartYear)
	require.Equal(t, 24, res.EpisodeLengthMin)
	require.Equal(t, "abc123xyz", res.TrailerYouTubeID)
	require.Equal(t, "Pilot", res.EpisodeTitleByNum[1])
	require.Equal(t, "The Chase", res.EpisodeTitleByNum[2])
}
