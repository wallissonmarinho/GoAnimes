package stremio

import (
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// TheTVDBAttributionLine is the in-app footer for series meta (free-tier attribution; link required by TheTVDB).
// Ref: https://www.thetvdb.com/api-information#attribution
const TheTVDBAttributionLine = "Metadados em parte do TheTVDB (https://www.thetvdb.com/). Podes complementar informações no site ou subscrever."

// MetaUsesTheTVDBData is true when stored enrichment clearly includes TVDB-sourced fields (id, artwork, or episode thumbs).
func MetaUsesTheTVDBData(en domain.SeriesEnrichment) bool {
	if en.TvdbSeriesID > 0 {
		return true
	}
	u := strings.ToLower(strings.TrimSpace(en.StremioHeroBackgroundURL))
	if strings.Contains(u, "thetvdb.com") {
		return true
	}
	for _, thumb := range en.EpisodeThumbnailByNum {
		if strings.Contains(strings.ToLower(strings.TrimSpace(thumb)), "thetvdb.com") {
			return true
		}
	}
	return false
}

// AppendTheTVDBAttributionToDescription appends the required attribution when body is non-empty; otherwise returns the line alone.
func AppendTheTVDBAttributionToDescription(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return TheTVDBAttributionLine
	}
	return body + "\n\n— " + TheTVDBAttributionLine
}
