package ginapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	ginapi "github.com/wallissonmarinho/GoAnimes/internal/adapters/http/ginapi"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
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
	store.Set(snap)
	serID := snap.Series[0].ID
	vid := domain.EpisodeVideoStremioID(serID, 1, 1, false)

	e := gin.New()
	e.Use(ginapi.CorsMiddleware())
	e.Use(gin.Recovery())
	ginapi.Register(e, ginapi.Config{AdminAPIKey: ""}, ginapi.Deps{
		Store: store,
		Log:   nil,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/catalog/anime/goanimes.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"metas"`)
	require.Contains(t, w.Body.String(), `"type":"series"`)
	require.Contains(t, w.Body.String(), serID)
	require.Contains(t, w.Body.String(), "Test Show")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/meta/series/"+serID+".json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"meta"`)
	require.Contains(t, w.Body.String(), `"videos"`)
	require.Contains(t, w.Body.String(), vid)
	require.NotContains(t, w.Body.String(), "goanimes:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.Contains(t, w.Body.String(), `"title":"E1"`)
	require.Contains(t, w.Body.String(), `"released"`)
	require.Regexp(t, `"released":"\d{4}-\d{2}-\d{2}T`, w.Body.String())

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
}

func TestCORSPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(ginapi.CorsMiddleware())
	e.Use(gin.Recovery())
	ginapi.Register(e, ginapi.Config{}, ginapi.Deps{Store: &state.CatalogStore{}})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/manifest.json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}
