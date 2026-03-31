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

// MergeAniListEnrichment fills empty fields in stored with values from add (e.g. DB row + lazy AniList fetch).
func MergeAniListEnrichment(stored, add AniListSeriesEnrichment) AniListSeriesEnrichment {
	out := stored
	if strings.TrimSpace(out.Description) == "" {
		out.Description = strings.TrimSpace(add.Description)
	}
	if len(out.Genres) == 0 && len(add.Genres) > 0 {
		out.Genres = append([]string(nil), add.Genres...)
	}
	if strings.TrimSpace(out.PosterURL) == "" {
		out.PosterURL = strings.TrimSpace(add.PosterURL)
	}
	if strings.TrimSpace(out.BackgroundURL) == "" {
		out.BackgroundURL = strings.TrimSpace(add.BackgroundURL)
	}
	if out.StartYear == 0 && add.StartYear > 0 {
		out.StartYear = add.StartYear
	}
	if out.EpisodeLengthMin == 0 && add.EpisodeLengthMin > 0 {
		out.EpisodeLengthMin = add.EpisodeLengthMin
	}
	if strings.TrimSpace(out.TrailerYouTubeID) == "" {
		out.TrailerYouTubeID = strings.TrimSpace(add.TrailerYouTubeID)
	}
	if strings.TrimSpace(out.TitlePreferred) == "" {
		out.TitlePreferred = strings.TrimSpace(add.TitlePreferred)
	}
	if add.EpisodeTitleByNum != nil {
		if out.EpisodeTitleByNum == nil {
			out.EpisodeTitleByNum = make(map[int]string)
		}
		for k, v := range add.EpisodeTitleByNum {
			if _, ok := out.EpisodeTitleByNum[k]; !ok {
				out.EpisodeTitleByNum[k] = v
			}
		}
	}
	return out
}
