package ginapi

import (
	"errors"
	"io"
	"net/http"
	"math"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

func (h *handlers) registerGoaiAuditRoutes(admin *gin.RouterGroup) {
	admin.GET("/goai-audit/series", h.getGoaiSeriesAudits)
	admin.POST("/goai-audit/series/:id/reaudit", h.postGoaiSeriesReaudit)
}

func (h *handlers) getGoaiSeriesAudits(c *gin.Context) {
	svc := h.deps.GoaiAuditAdmin
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goai audit admin not configured"})
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
	confMin, err := parseConfidenceBound(c.Query("confidence_min"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid confidence_min (use number between 0 and 1)"})
		return
	}
	confMax, err := parseConfidenceBound(c.Query("confidence_max"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid confidence_max (use number between 0 and 1)"})
		return
	}
	if confMin != nil && confMax != nil && *confMin > *confMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confidence_min cannot be greater than confidence_max"})
		return
	}
	ctx := c.Request.Context()
	page, err := svc.ListSeriesAudits(ctx, domain.GoaiAuditListParams{
		Limit:         limit,
		Offset:        offset,
		ConfidenceMin: confMin,
		ConfidenceMax: confMax,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

func parseConfidenceBound(raw string) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		return nil, strconv.ErrSyntax
	}
	return &v, nil
}

// postGoaiSeriesReauditBody optional JSON: scope controls whether release rows are cleared.
// full|default|"" — delete goai_release_audit for the series then set needs_reaudit.
// series_only|flag_only — only SetSeriesNeedsReaudit (keep cached release audits).
type postGoaiSeriesReauditBody struct {
	Scope string `json:"scope,omitempty"`
}

func (h *handlers) postGoaiSeriesReaudit(c *gin.Context) {
	svc := h.deps.GoaiAuditAdmin
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goai audit admin not configured"})
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
	ctx := c.Request.Context()
	out, err := svc.RequestSeriesReaudit(ctx, domain.GoaiSeriesReauditRequest{
		SeriesID: seriesID,
		Scope:    body.Scope,
	})
	if err != nil {
		if errors.Is(err, services.ErrGoaiAuditInvalidScope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope (use full, default, series_only, or flag_only)"})
			return
		}
		if errors.Is(err, services.ErrGoaiAuditSeriesAbsent) {
			c.JSON(http.StatusNotFound, gin.H{"error": "series has no goai_series_audit row yet"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, struct {
		Status string `json:"status"`
		domain.GoaiSeriesReauditResult
	}{
		Status:                  "accepted",
		GoaiSeriesReauditResult: out,
	})
}
