package domain

import "strings"

// AniListSeriesEnrichment is cached Stremio-facing metadata from AniList (per series id).
type AniListSeriesEnrichment struct {
	PosterURL         string            `json:"poster,omitempty"`
	BackgroundURL     string            `json:"background,omitempty"`
	Description       string            `json:"description,omitempty"`
	Genres            []string          `json:"genres,omitempty"`
	StartYear         int               `json:"start_year,omitempty"`
	EpisodeLengthMin  int               `json:"ep_min,omitempty"`
	TrailerYouTubeID  string            `json:"trailer_yt,omitempty"`
	TitlePreferred    string            `json:"title_pref,omitempty"`
	EpisodeTitleByNum map[int]string    `json:"ep_titles"`
}

// AniListNeedsRefetch is true when we should call AniList again (missing data or legacy poster-only row).
func AniListNeedsRefetch(en AniListSeriesEnrichment) bool {
	if strings.TrimSpace(en.PosterURL) == "" {
		return true
	}
	if strings.TrimSpace(en.Description) == "" {
		return true
	}
	// nil = snapshot row from before episode titles were stored; fetch once to populate streamingEpisodes.
	if en.EpisodeTitleByNum == nil {
		return true
	}
	return false
}
