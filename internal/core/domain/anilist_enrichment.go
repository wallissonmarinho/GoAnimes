package domain

import "strings"

// AniListSeriesEnrichment is cached Stremio-facing metadata from AniList (per series id).
type AniListSeriesEnrichment struct {
	PosterURL        string   `json:"poster,omitempty"`
	BackgroundURL    string   `json:"background,omitempty"`
	Description      string   `json:"description,omitempty"`
	Genres           []string `json:"genres,omitempty"`
	StartYear        int      `json:"start_year,omitempty"`
	EpisodeLengthMin int      `json:"ep_min,omitempty"`
	TrailerYouTubeID string   `json:"trailer_yt,omitempty"`
	TitlePreferred   string   `json:"title_pref,omitempty"`
}

// AniListNeedsRefetch is true when we should call AniList again (missing data or legacy poster-only row).
func AniListNeedsRefetch(en AniListSeriesEnrichment) bool {
	if strings.TrimSpace(en.PosterURL) == "" {
		return true
	}
	if strings.TrimSpace(en.Description) == "" {
		return true
	}
	return false
}
