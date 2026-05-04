package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wallissonmarinho/GoAnimes/internal/app/admin"
	"github.com/wallissonmarinho/GoAnimes/internal/app/stremio"
	syncsvc "github.com/wallissonmarinho/GoAnimes/internal/app/sync"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

type Deps struct {
	Stremio  *stremio.Service
	Sync     *syncsvc.Service
	Admin    *admin.Service
	AdminKey string
}

type handlers struct {
	deps Deps
}

func Register(engine *gin.Engine, deps Deps) {
	h := &handlers{deps: deps}
	engine.GET("/health", h.health)
	engine.HEAD("/health", h.headHealth)

	engine.GET("/manifest.json", h.manifest)
	engine.GET("/catalog/:type/*catalogPath", h.catalog)
	engine.GET("/meta/:type/:meta_id", h.meta)
	engine.GET("/stream/:type/:stream_id", h.stream)

	adminGroup := engine.Group("/admin")
	adminGroup.Use(h.requireAdminKey)
	adminGroup.POST("/sync", h.sync)
	adminGroup.GET("/feeds", h.listFeeds)
	adminGroup.POST("/feeds", h.createFeed)
	adminGroup.PUT("/feeds/:id", h.updateFeed)
	adminGroup.DELETE("/feeds/:id", h.deleteFeed)
	adminGroup.GET("/mapping-overrides", h.listOverrides)
	adminGroup.POST("/mapping-overrides", h.upsertOverride)
	adminGroup.GET("/unmatched", h.listUnmatched)
}

func (h *handlers) health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (h *handlers) headHealth(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (h *handlers) manifest(c *gin.Context) {
	payload, err := h.deps.Stremio.Manifest(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, payload)
}

func (h *handlers) catalog(c *gin.Context) {
	if c.Param("type") != stremio.StremioType {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	catalogID, extras, ok := stremio.ParseCatalogPath(c.Param("catalogPath"))
	if !ok || (catalogID != stremio.CatalogIDMain && catalogID != stremio.CatalogIDWeek) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	limit := 80
	skip := 0
	if v := strings.TrimSpace(extras["skip"]); v != "" {
		skip = atoi(v)
	}
	metas, err := h.deps.Stremio.Catalog(c.Request.Context(), catalogID, extras, limit, skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"metas": metas})
}

func (h *handlers) meta(c *gin.Context) {
	if c.Param("type") != stremio.StremioType {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	id := strings.TrimSuffix(c.Param("meta_id"), ".json")
	meta, found, err := h.deps.Stremio.Meta(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	c.JSON(http.StatusOK, gin.H{"meta": meta})
}

func (h *handlers) stream(c *gin.Context) {
	if c.Param("type") != stremio.StremioType {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	id := strings.TrimSuffix(c.Param("stream_id"), ".json")
	streams, err := h.deps.Stremio.Streams(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"streams": streams})
}

func (h *handlers) sync(c *gin.Context) {
	res := h.deps.Sync.Run(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"started_at":  res.StartedAt,
		"finished_at": res.FinishedAt,
		"processed":   res.Processed,
		"errors":      errorsToStrings(res.Errors),
	})
}

func (h *handlers) listFeeds(c *gin.Context) {
	feeds, err := h.deps.Admin.ListFeeds(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"feeds": feeds})
}

func (h *handlers) createFeed(c *gin.Context) {
	var input struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and url are required"})
		return
	}
	feed := domain.Feed{
		Name: input.Name, URL: input.URL, Type: domain.FeedType(input.Type), Enabled: input.Enabled,
	}
	out, err := h.deps.Admin.UpsertFeed(c.Request.Context(), feed)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"feed": out})
}

func (h *handlers) updateFeed(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	var input struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and url are required"})
		return
	}
	feed := domain.Feed{
		ID: id, Name: input.Name, URL: input.URL, Type: domain.FeedType(input.Type), Enabled: input.Enabled,
	}
	out, err := h.deps.Admin.UpsertFeed(c.Request.Context(), feed)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"feed": out})
}

func (h *handlers) deleteFeed(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if err := h.deps.Admin.DeleteFeed(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handlers) listOverrides(c *gin.Context) {
	overrides, err := h.deps.Admin.ListOverrides(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"overrides": overrides})
}

func (h *handlers) upsertOverride(c *gin.Context) {
	var input struct {
		ID         string `json:"id"`
		RSSNameKey string `json:"rss_name_key"`
		TMDBID     int    `json:"tmdb_id"`
		Season     int    `json:"season"`
		Locked     bool   `json:"locked"`
	}
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	override := domain.MappingOverride{
		ID: input.ID, RSSNameKey: input.RSSNameKey, TMDBID: input.TMDBID, Season: input.Season, Locked: input.Locked,
	}
	out, err := h.deps.Admin.UpsertOverride(c.Request.Context(), override)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"override": out})
}

func (h *handlers) listUnmatched(c *gin.Context) {
	limit := atoi(c.Query("limit"))
	items, err := h.deps.Admin.ListUnmatched(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unmatched": items})
}

func (h *handlers) requireAdminKey(c *gin.Context) {
	if h.deps.AdminKey == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "admin key not configured"})
		return
	}
	key := strings.TrimSpace(c.GetHeader("X-Admin-Key"))
	if key == "" {
		auth := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			key = strings.TrimSpace(auth[7:])
		}
	}
	if key != h.deps.AdminKey {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid admin key"})
		return
	}
	c.Next()
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func errorsToStrings(errs []error) []string {
	if len(errs) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}
