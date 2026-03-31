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
				ID:           "goanimes:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
				Type:         "movie",
				Name:         "[Torrent] Test Show - 01 [1080p CR WEB-DL AVC AAC][us][br]",
				InfoHash:     "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
				SubtitlesTag: "[br]",
			},
		},
	}
	domain.EnsureSnapshotGrouped(&snap)
	store.Set(snap)
	serID := snap.Series[0].ID
	epID := snap.Items[0].ID

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
	require.Contains(t, w.Body.String(), epID)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/stream/series/"+epID+".json", nil)
	e.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"streams"`)
	require.Contains(t, w.Body.String(), "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

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
