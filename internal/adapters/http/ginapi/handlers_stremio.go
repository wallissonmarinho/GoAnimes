package ginapi

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const (
	catalogStremioID = "goanimes"
	// Catalog is listed under Discover → anime (same URL pattern as Kitsu).
	stremioTypeAnime   = "anime"
	stremioTypeMovie   = "movie"
	stremioTypeSeries  = "series"
)

func stremioMetaOrStreamTypeOK(t string) bool {
	switch t {
	case stremioTypeAnime, stremioTypeMovie, "series":
		return true
	default:
		return false
	}
}

// Stremio requires each series video object to include "released" as ISO 8601; missing → "no metadata" in the app.
func stremioVideoReleasedISO(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "1970-01-01T00:00:00.000Z"
	}
	if strings.Contains(s, "T") {
		return s
	}
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s + "T12:00:00.000Z"
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.UnixDate, "Mon, 02 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return "1970-01-01T00:00:00.000Z"
}

func stremioUnescapePathParam(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

func (h *handlers) getManifest(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":          "org.goanimes",
		"version":     "1.0.4",
		"name":        "GoAnimes",
		"description": "RSS anime torrents with pt-BR (Erai [br]) filter",
		"types":       []string{stremioTypeAnime, stremioTypeMovie, stremioTypeSeries},
		"catalogs": []gin.H{
			{"type": stremioTypeAnime, "id": catalogStremioID, "name": "GoAnimes"},
		},
		"resources": []any{
			"catalog",
			gin.H{"name": "meta", "types": []string{stremioTypeAnime, stremioTypeMovie, stremioTypeSeries}, "idPrefixes": []string{rss.StremioIDPrefix}},
			gin.H{"name": "stream", "types": []string{stremioTypeAnime, stremioTypeMovie, stremioTypeSeries}, "idPrefixes": []string{rss.StremioIDPrefix}},
		},
		"idPrefixes": []string{rss.StremioIDPrefix},
	})
}

func (h *handlers) getCatalog(c *gin.Context) {
	typ := c.Param("type")
	cid := strings.TrimSuffix(c.Param("catalog_id"), ".json")
	if typ != stremioTypeAnime || cid != catalogStremioID {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	snap := h.deps.Store.Snapshot()
	metas := make([]gin.H, 0, len(snap.Series))
	for _, s := range snap.Series {
		m := gin.H{
			"id":     s.ID,
			"type":   stremioTypeSeries,
			"name":   s.Name,
			"genres": []string{"Anime"},
		}
		if s.Poster != "" {
			m["poster"] = s.Poster
		}
		metas = append(metas, m)
	}
	c.JSON(http.StatusOK, gin.H{"metas": metas})
}

func (h *handlers) getMeta(c *gin.Context) {
	typ := c.Param("type")
	id := stremioUnescapePathParam(strings.TrimSuffix(c.Param("meta_id"), ".json"))
	if !stremioMetaOrStreamTypeOK(typ) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	// Discover is under "anime"; some clients still request /meta/anime/:id for series rows.
	if domain.IsSeriesStremioID(id) {
		ser, ok := h.deps.Store.SeriesByID(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{})
			return
		}
		eps := h.deps.Store.ItemsBySeriesID(id)
		videos := make([]gin.H, 0, len(eps))
		for _, it := range eps {
			epNum := it.Episode
			if it.IsSpecial {
				epNum = 0
			}
			v := gin.H{
				"id":       it.ID,
				"title":    domain.EpisodeVideoTitle(it),
				"released": stremioVideoReleasedISO(it.Released),
				"season":   it.Season,
				"episode":  epNum,
			}
			videos = append(videos, v)
		}
		meta := gin.H{
			"id":          ser.ID,
			"type":        stremioTypeSeries,
			"name":        ser.Name,
			"poster":      ser.Poster,
			"genres":      []string{"Anime"},
			"description": "Torrent releases with pt-BR subtitles (Erai).",
			"videos":      videos,
		}
		c.JSON(http.StatusOK, gin.H{"meta": meta})
		return
	}
	it, ok := h.deps.Store.ItemByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	meta := gin.H{
		"id":   it.ID,
		"type": it.Type,
		"name": it.Name,
	}
	if it.Poster != "" {
		meta["poster"] = it.Poster
	}
	if it.Released != "" {
		meta["releaseInfo"] = it.Released
	}
	if it.SubtitlesTag != "" {
		meta["description"] = "Subtitles: " + it.SubtitlesTag
	}
	c.JSON(http.StatusOK, gin.H{"meta": meta})
}

func (h *handlers) getStream(c *gin.Context) {
	typ := c.Param("type")
	id := stremioUnescapePathParam(strings.TrimSuffix(c.Param("stream_id"), ".json"))
	if !stremioMetaOrStreamTypeOK(typ) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	it, ok := h.deps.Store.ItemByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	var streams []gin.H
	if it.InfoHash != "" {
		streams = append(streams, gin.H{
			"name":     "Torrent",
			"title":    it.Name,
			"infoHash": it.InfoHash,
			"fileIdx":  0,
			"behaviorHints": gin.H{
				"bingeGroup": it.ID,
			},
		})
	} else if it.MagnetURL != "" {
		streams = append(streams, gin.H{
			"name": "Magnet",
			"title": it.Name,
			"url":  it.MagnetURL,
		})
	} else if it.TorrentURL != "" {
		streams = append(streams, gin.H{
			"name": "Torrent file",
			"title": it.Name,
			"url":  it.TorrentURL,
		})
	}
	if len(streams) == 0 {
		c.JSON(http.StatusOK, gin.H{"streams": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"streams": streams})
}

func (h *handlers) getHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
