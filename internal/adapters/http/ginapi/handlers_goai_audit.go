package ginapi

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *handlers) registerGoaiAuditRoutes(admin *gin.RouterGroup) {
	admin.GET("/goai-audit/series", h.getGoaiSeriesAudits)
	admin.POST("/goai-audit/series/:id/reaudit", h.postGoaiSeriesReaudit)
}

func (h *handlers) getGoaiSeriesAudits(c *gin.Context) {
	repo := h.deps.GoaiAuditRepo
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goai audit repository not configured"})
		return
	}
	limit := 50
	offset := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	ctx := c.Request.Context()
	items, err := repo.ListSeriesAuditsForAdmin(ctx, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "limit": limit, "offset": offset})
}

// postGoaiSeriesReauditBody optional JSON: scope controls whether release rows are cleared.
// full|default|"" — delete goai_release_audit for the series then set needs_reaudit.
// series_only|flag_only — only SetSeriesNeedsReaudit (keep cached release audits).
type postGoaiSeriesReauditBody struct {
	Scope string `json:"scope,omitempty"`
}

func parseReauditScope(scope string) (fullClear bool, ok bool) {
	s := strings.TrimSpace(strings.ToLower(scope))
	switch s {
	case "", "full", "default":
		return true, true
	case "series_only", "flag_only":
		return false, true
	default:
		return false, false
	}
}

func (h *handlers) postGoaiSeriesReaudit(c *gin.Context) {
	repo := h.deps.GoaiAuditRepo
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goai audit repository not configured"})
		return
	}
	seriesID := strings.TrimSpace(c.Param("id"))
	if seriesID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "series id required in path"})
		return
	}
	var body postGoaiSeriesReauditBody
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil && err != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
			return
		}
	}
	fullClear, valid := parseReauditScope(body.Scope)
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope (use full, default, series_only, or flag_only)"})
		return
	}
	ctx := c.Request.Context()
	if fullClear {
		if err := repo.DeleteReleaseAuditsForSeries(ctx, seriesID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := repo.SetSeriesNeedsReaudit(ctx, seriesID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "series has no goai_series_audit row yet"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"status":                 "accepted",
		"series_id":              seriesID,
		"cleared_release_audits": fullClear,
	})
}
