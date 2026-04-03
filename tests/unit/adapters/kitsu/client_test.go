package kitsu_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/kitsu"
)

func TestClient_SearchAnimeEnrichment(t *testing.T) {
	const payload = `{
		"data": [{
			"id": "42",
			"type": "anime",
			"attributes": {
				"synopsis": "A <b>fine</b> space story.",
				"canonicalTitle": "Romanized",
				"titles": {"en": "English Title", "ja_jp": "ネイティブ"},
				"startDate": "2025-04-01",
				"episodeLength": 24,
				"posterImage": {
					"original": "https://cdn.example/p.jpg",
					"meta": {"dimensions": {"tiny": {"width": 1, "height": 2}}}
				},
				"coverImage": {"large": "https://cdn.example/c.jpg", "meta": {}}
			},
			"relationships": {
				"categories": {
					"data": [{"type": "categories", "id": "7"}]
				}
			}
		}],
		"included": [
			{"id": "7", "type": "categories", "attributes": {"title": "Action"}}
		]
	}`

	mux := http.NewServeMux()
	mux.HandleFunc("/api/edge/anime", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/vnd.api+json", r.Header.Get("Accept"))
		require.NotEmpty(t, r.URL.Query().Get("filter[text]"))
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(payload))
	})
	mux.HandleFunc("/api/edge/episodes", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "42", r.URL.Query().Get("filter[mediaId]"))
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "ep1",
				"type": "episodes",
				"attributes": {
					"number": 1,
					"canonicalTitle": "From Kitsu",
					"thumbnail": {"original": "https://cdn.example/kitsu-ep1.png"}
				}
			}],
			"meta": {"count": 1}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	g := httpclient.NewGetter(5*time.Second, "GoAnimes/test", 1<<20)
	c := kitsu.NewClient(g, kitsu.WithBaseURL(srv.URL+"/api/edge"), kitsu.WithMinRequestInterval(0))

	en, err := c.SearchAnimeEnrichment(context.Background(), "Test Query")
	require.NoError(t, err)
	require.Equal(t, "English Title", en.TitlePreferred)
	require.Equal(t, "ネイティブ", en.TitleNative)
	require.Contains(t, en.Description, "fine")
	require.Equal(t, 2025, en.StartYear)
	require.Equal(t, 24, en.EpisodeLengthMin)
	require.Equal(t, "https://cdn.example/p.jpg", en.PosterURL)
	require.Equal(t, "https://cdn.example/c.jpg", en.BackgroundURL)
	require.Equal(t, []string{"Ação"}, en.Genres)
	require.Equal(t, "42", en.KitsuAnimeID)

	g2 := httpclient.NewGetter(5*time.Second, "GoAnimes/test", 1<<20)
	c2 := kitsu.NewClient(g2, kitsu.WithBaseURL(srv.URL+"/api/edge"), kitsu.WithMinRequestInterval(0))
	titles, thumbs, err := c2.FetchEpisodeMaps(context.Background(), "42")
	require.NoError(t, err)
	require.Equal(t, "From Kitsu", titles[1])
	require.Equal(t, "https://cdn.example/kitsu-ep1.png", thumbs[1])
}
