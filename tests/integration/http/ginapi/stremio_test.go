package ginapi_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	ginapi "github.com/wallissonmarinho/GoAnimes/internal/adapters/http/ginapi"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

func TestStremioRoutes_catalogMetaStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &state.CatalogStore{}
	snap := domain.CatalogSnapshot{
		OK: true,
		Items: []domain.CatalogItem{
			{
				ID:           "goanimes:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Type:         "movie",
				Name:         "[Torrent] Test Show - 01 [720p CR WEB-DL AVC AAC][us][br]",
				InfoHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SubtitlesTag: "[br]",
			},
			{
				ID:           "goanimes:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Type:         "movie",
				Name:         "[Torrent] Test Show - 01 [1080p CR WEB-DL AVC AAC][us][br]",
				InfoHash:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				SubtitlesTag: "[br]",
			},
		},
	}
	domain.EnsureSnapshotGrouped(&snap)
	serID := snap.Series[0].ID
	snap.AniListBySeries = map[string]domain.AniListSeriesEnrichment{
		serID: {
			PosterURL:      "https://cdn.example/poster.jpg",
			Description:    "A test synopsis for catalog.",
			Genres:         []string{"Comedy"},
			StartYear:      2020,
			TitlePreferred: "Test Show",
			TitleNative:    "試験ショー",
			EpisodeTitleByNum: map[int]string{},
		},
	}
	domain.ApplyAniListEnrichmentToSeries(&snap)
	store.Set(snap)
	vid := domain.EpisodeVideoStremioID(serID, 1, 1, false)

	e := gin.New()
	e.Use(ginapi.CorsMiddleware())
	e.Use(gin.Recovery())
	ginapi.Register(e, ginapi.Config{AdminAPIKey: ""}, ginapi.Deps{
		Catalog: services.NewCatalogAdminService(nil, store),
		Log:     nil,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/catalog/anime/goanimes.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"metas"`)
	require.Contains(t, w.Body.String(), `"type":"anime"`)
	require.Contains(t, w.Body.String(), serID)
	require.Contains(t, w.Body.String(), "Test Show")
	require.NotContains(t, w.Body.String(), "試験ショー")
	require.Contains(t, w.Body.String(), "A test synopsis for catalog.")
	require.Contains(t, w.Body.String(), "2020-")
	require.Contains(t, w.Body.String(), "Comédia")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/meta/series/"+serID+".json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"meta"`)
	require.Contains(t, w.Body.String(), `"videos"`)
	require.Contains(t, w.Body.String(), vid)
	require.NotContains(t, w.Body.String(), "goanimes:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.Contains(t, w.Body.String(), `"title":"E1 ·`)
	require.Contains(t, w.Body.String(), "1080p")
	require.Contains(t, w.Body.String(), `"released"`)
	require.Regexp(t, `"released":"\d{4}-\d{2}-\d{2}T`, w.Body.String())
	require.Contains(t, w.Body.String(), "Test Show")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/stream/series/"+vid+".json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"streams"`)
	require.Contains(t, w.Body.String(), "Torrent · 1080p")
	require.Contains(t, w.Body.String(), "Torrent · 720p")
	require.Contains(t, w.Body.String(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.Contains(t, w.Body.String(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "org.goanimes")
	require.Contains(t, w.Body.String(), "goanimes-week")
	require.Contains(t, w.Body.String(), `"name":"genre"`)
}

func TestStremioCatalog_weekAndGenreFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &state.CatalogStore{}
	today := time.Now().UTC().Format("2006-01-02")
	old := time.Now().UTC().AddDate(0, 0, -14).Format("2006-01-02")
	snap := domain.CatalogSnapshot{
		OK: true,
		Items: []domain.CatalogItem{
			{ID: "goanimes:recent", Name: "[Torrent] New Hot - 01 [720p][br]", Released: today},
			{ID: "goanimes:oldonly", Name: "[Torrent] Old Only - 01 [720p][br]", Released: old},
		},
	}
	domain.EnsureSnapshotGrouped(&snap)
	var newID, oldID string
	for i := range snap.Series {
		switch snap.Series[i].Name {
		case "New Hot":
			newID = snap.Series[i].ID
			snap.Series[i].Genres = []string{"Comédia"}
		case "Old Only":
			oldID = snap.Series[i].ID
			snap.Series[i].Genres = []string{"Ação"}
		}
	}
	require.NotEmpty(t, newID)
	require.NotEmpty(t, oldID)
	store.Set(snap)

	e := gin.New()
	e.Use(ginapi.CorsMiddleware())
	e.Use(gin.Recovery())
	ginapi.Register(e, ginapi.Config{}, ginapi.Deps{Catalog: services.NewCatalogAdminService(nil, store)})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/catalog/anime/goanimes-week.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), newID)
	require.NotContains(t, w.Body.String(), oldID)

	genrePath := "/catalog/anime/goanimes/genre=" + url.PathEscape("Comédia") + ".json"
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, genrePath, nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), newID)
	require.NotContains(t, w.Body.String(), oldID)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/catalog/anime/unknown.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestCORSPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(ginapi.CorsMiddleware())
	e.Use(gin.Recovery())
	ginapi.Register(e, ginapi.Config{}, ginapi.Deps{Catalog: services.NewCatalogAdminService(nil, &state.CatalogStore{})})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/manifest.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}
