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
						"title": {"userPreferred": "試験ユーザー優先", "english": "Test Anime EN", "native": "試験アニメ", "romaji": "Test Anime Romaji"},
						"coverImage": {"extraLarge": "https://cdn.example/poster.jpg", "large": ""},
						"bannerImage": "https://cdn.example/banner.jpg",
						"description": "A <b>fine</b> show.",
						"genres": ["Action", "Comedy"],
						"seasonYear": 2024,
						"startDate": {"year": 2024},
						"duration": 24,
						"trailer": {"id": "abc123xyz", "site": "youtube"},
						"nextAiringEpisode": {"airingAt": 1735689600, "episode": 5},
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
	c := anilist.NewClient(g, anilist.WithEndpoint(srv.URL), anilist.WithMinRequestInterval(0))

	res, err := c.SearchAnimeMedia(context.Background(), "Test Anime")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/poster.jpg", res.PosterURL)
	require.Equal(t, "https://cdn.example/banner.jpg", res.BackgroundURL)
	require.Equal(t, "Test Anime Romaji", res.Title)
	require.Equal(t, "試験アニメ", res.NativeTitle)
	require.Contains(t, res.Description, "fine")
	require.Equal(t, []string{"Action", "Comedy"}, res.Genres)
	require.Equal(t, 2024, res.StartYear)
	require.Equal(t, 24, res.EpisodeLengthMin)
	require.Equal(t, "abc123xyz", res.TrailerYouTubeID)
	require.Equal(t, "Pilot", res.EpisodeTitleByNum[1])
	require.Equal(t, "The Chase", res.EpisodeTitleByNum[2])
	require.Equal(t, int64(1735689600), res.NextAiringUnix)
	require.Equal(t, 5, res.NextAiringEpisode)
}

func TestClient_SearchAnimeMedia_picksBestMatchAmongResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Page": {
					"media": [
						{
							"title": {"romaji": "Solo Leveling Season 2", "english": "Solo Leveling Season 2", "native": "", "userPreferred": "Solo Leveling Season 2"},
							"coverImage": {"extraLarge": "https://cdn.example/solo.jpg", "large": ""},
							"description": "Wrong hit",
							"genres": ["Action"],
							"seasonYear": 2025,
							"startDate": {"year": 2025},
							"duration": 24,
							"streamingEpisodes": []
						},
						{
							"title": {"romaji": "Dorohedoro Season 2", "english": "Dorohedoro Season 2", "native": "", "userPreferred": "Dorohedoro Season 2"},
							"coverImage": {"extraLarge": "https://cdn.example/dorohedoro.jpg", "large": ""},
							"description": "Right show",
							"genres": ["Action", "Fantasy"],
							"seasonYear": 2025,
							"startDate": {"year": 2025},
							"duration": 24,
							"streamingEpisodes": []
						},
						{
							"title": {"romaji": "Mob Psycho 100 III", "english": "Mob Psycho 100 III", "native": "", "userPreferred": "Mob Psycho 100 III"},
							"coverImage": {"extraLarge": "https://cdn.example/mob.jpg", "large": ""},
							"description": "Other",
							"genres": ["Comedy"],
							"seasonYear": 2022,
							"startDate": {"year": 2022},
							"duration": 24,
							"streamingEpisodes": []
						}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	g := httpclient.NewGetter(5*time.Second, "GoAnimes/test", 1<<20)
	c := anilist.NewClient(g, anilist.WithEndpoint(srv.URL), anilist.WithMinRequestInterval(0))

	res, err := c.SearchAnimeMedia(context.Background(), "Dorohedoro Season 2")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/dorohedoro.jpg", res.PosterURL)
	require.Equal(t, "Dorohedoro Season 2", res.Title)
	require.Contains(t, res.Description, "Right show")
}
