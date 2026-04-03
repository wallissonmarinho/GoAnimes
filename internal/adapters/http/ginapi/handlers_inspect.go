package ginapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// Inspect routes expose the same catalog/meta facts Stremio uses, in one JSON for tooling and QA.
// Stremio itself calls the public URLs below (no /api prefix); these inspect endpoints sit under /api/v1 and follow admin auth.

func (h *handlers) registerInspectRoutes(admin *gin.RouterGroup) {
	admin.GET("/inspect/catalog", h.getInspectCatalog)
	admin.GET("/inspect/series", h.getInspectSeries)
}

func (h *handlers) getInspectCatalog(c *gin.Context) {
	snap := h.deps.Store.Snapshot()
	seriesOut := make([]gin.H, 0, len(snap.Series))
	for _, s := range snap.Series {
		en := snap.AniListBySeries[s.ID]
		groups := domain.GroupItemsByEpisode(snap.Items, s.ID)
		keys := domain.OrderedEpisodeKeys(groups)
		bySeason := make(map[int]int)
		for _, k := range keys {
			bySeason[k.Season]++
		}
		seriesOut = append(seriesOut, gin.H{
			"id":                 s.ID,
			"name":               s.Name,
			"poster":             s.Poster,
			"description":        s.Description,
			"genres":             s.Genres,
			"release_info":       s.ReleaseInfo,
			"distinct_episodes":  len(keys),
			"torrent_releases":   countItemsForSeries(snap.Items, s.ID),
			"episodes_by_season": bySeason,
			"enrichment": gin.H{
				"title_preferred":      en.TitlePreferred,
				"title_native":         en.TitleNative,
				"ani_list_search_ver":  en.AniListSearchVer,
				"poster_url":           en.PosterURL,
				"background_url":       en.BackgroundURL,
				"description_len":      len(strings.TrimSpace(en.Description)),
				"genres":               en.Genres,
				"start_year":           en.StartYear,
				"episode_length_min":   en.EpisodeLengthMin,
				"episode_titles_count": len(en.EpisodeTitleByNum),
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":          snap.OK,
		"message":     snap.Message,
		"item_count":  snap.ItemCount,
		"series_count": len(snap.Series),
		"stremio_public_urls": gin.H{
			"manifest": "/manifest.json",
			"catalog":  "/" + strings.Join([]string{"catalog", stremioTypeAnime, catalogStremioID + ".json"}, "/"),
			"meta":     "/" + strings.Join([]string{"meta", stremioTypeAnime, "{series_id}.json"}, "/"),
			"stream":   "/" + strings.Join([]string{"stream", stremioTypeAnime, "{video_id}.json"}, "/"),
			"note":     "Replace {series_id} with path-encoded id, e.g. " + url.PathEscape("goanimes:series:abc") + ".json",
		},
		"series": seriesOut,
	})
}

func countItemsForSeries(items []domain.CatalogItem, seriesID string) int {
	n := 0
	for _, it := range items {
		if it.SeriesID == seriesID {
			n++
		}
	}
	return n
}

func (h *handlers) getInspectSeries(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id query parameter (Stremio series id, e.g. goanimes:series:…)"})
		return
	}
	snap := h.deps.Store.Snapshot()
	ser, ok := h.deps.Store.SeriesByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found", "id": id})
		return
	}
	en := snap.AniListBySeries[id]
	searchHint := domain.AniListSearchQueryFromItems(snap.Items, id)

	groups := domain.GroupItemsByEpisode(snap.Items, id)
	keys := domain.OrderedEpisodeKeys(groups)
	episodes := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		group := groups[k]
		sampleN := min(3, len(group))
		names := make([]string, 0, sampleN)
		for i := range sampleN {
			names = append(names, group[i].Name)
		}
		epNum := k.Episode
		if k.Special {
			epNum = 0
		}
		episodes = append(episodes, gin.H{
			"season":               k.Season,
			"episode":              epNum,
			"special":              k.Special,
			"torrent_variants":     len(group),
			"stremio_video_id":     domain.EpisodeVideoStremioID(id, k.Season, k.Episode, k.Special),
			"episode_list_title":   domain.EpisodeListTitle(k.Episode, k.Special, en.EpisodeTitleByNum),
			"sample_release_titles": names,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"series": gin.H{
			"id":            ser.ID,
			"name":          ser.Name,
			"poster":        ser.Poster,
			"description":   ser.Description,
			"genres":        ser.Genres,
			"release_info":  ser.ReleaseInfo,
		},
		"ani_list_search_hint": searchHint,
		"enrichment":           en,
		"episodes":             episodes,
		"stremio_meta_url": "/" + strings.Join([]string{"meta", stremioTypeAnime, url.PathEscape(id) + ".json"}, "/"),
	})
}
