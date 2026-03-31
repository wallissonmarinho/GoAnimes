package ginapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
)

const catalogStremioID = "goanimes"

func (h *handlers) getManifest(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"id":          "org.goanimes",
		"version":     "1.0.0",
		"name":        "GoAnimes",
		"description": "RSS anime torrents with pt-BR (Erai [br]) filter",
		"types":       []string{"movie"},
		"catalogs": []gin.H{
			{"type": "movie", "id": catalogStremioID, "name": "GoAnimes"},
		},
		"resources": []any{
			"catalog",
			gin.H{"name": "meta", "types": []string{"movie"}, "idPrefixes": []string{rss.StremioIDPrefix}},
			gin.H{"name": "stream", "types": []string{"movie"}, "idPrefixes": []string{rss.StremioIDPrefix}},
		},
		"idPrefixes": []string{rss.StremioIDPrefix},
	})
}

func (h *handlers) getCatalog(c *gin.Context) {
	typ := c.Param("type")
	cid := strings.TrimSuffix(c.Param("catalog_id"), ".json")
	if typ != "movie" || cid != catalogStremioID {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	snap := h.deps.Store.Snapshot()
	metas := make([]gin.H, 0, len(snap.Items))
	for _, it := range snap.Items {
		m := gin.H{
			"id":   it.ID,
			"type": it.Type,
			"name": it.Name,
		}
		if it.Poster != "" {
			m["poster"] = it.Poster
		}
		if it.Released != "" {
			m["releaseInfo"] = it.Released
		}
		metas = append(metas, m)
	}
	c.JSON(http.StatusOK, gin.H{"metas": metas})
}

func (h *handlers) getMeta(c *gin.Context) {
	typ := c.Param("type")
	id := strings.TrimSuffix(c.Param("meta_id"), ".json")
	if typ != "movie" {
		c.JSON(http.StatusNotFound, gin.H{})
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
	id := strings.TrimSuffix(c.Param("stream_id"), ".json")
	if typ != "movie" {
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
