package jikan_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/jikan"
)

func TestClient_SearchAnimeEnrichment(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v4/anime", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "1", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"mal_id":99}]}`))
	})
	mux.HandleFunc("/v4/anime/99", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/anime/99" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"title": "Original",
				"title_english": "English Title",
				"synopsis": "A <b>fine</b> story.",
				"year": 2025,
				"duration": "24 min per ep",
				"genres": [{"name": "Action"}],
				"images": {"jpg": {"large_image_url": "https://cdn.example/l.jpg"}},
				"trailer": {"youtube_id": "abc123xyz"}
			}
		}`))
	})
	mux.HandleFunc("/v4/anime/99/episodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"pagination": {"has_next_page": false},
			"data": [
				{"url": "https://myanimelist.net/anime/99/X/episode/1", "title": "First"},
				{"url": "https://myanimelist.net/anime/99/X/episode/2", "title": "Second"}
			]
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	g := httpclient.NewGetter(5*time.Second, "GoAnimes/test", 1<<20)
	c := jikan.NewClient(g, jikan.WithBaseURL(srv.URL+"/v4"))

	en, err := c.SearchAnimeEnrichment(context.Background(), "Test Query")
	require.NoError(t, err)
	require.Equal(t, "English Title", en.TitlePreferred)
	require.Contains(t, en.Description, "fine")
	require.Equal(t, []string{"Ação"}, en.Genres)
	require.Equal(t, 2025, en.StartYear)
	require.Equal(t, 24, en.EpisodeLengthMin)
	require.Equal(t, "https://cdn.example/l.jpg", en.PosterURL)
	require.Equal(t, "abc123xyz", en.TrailerYouTubeID)
	require.Equal(t, "First", en.EpisodeTitleByNum[1])
	require.Equal(t, "Second", en.EpisodeTitleByNum[2])
}
