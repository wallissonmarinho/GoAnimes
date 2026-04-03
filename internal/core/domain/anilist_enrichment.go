package domain

import (
	"strings"
	"time"
)

// AniListSeriesEnrichment is cached Stremio-facing metadata from AniList (per series id).
type AniListSeriesEnrichment struct {
	PosterURL         string         `json:"poster,omitempty"`
	BackgroundURL     string         `json:"background,omitempty"`
	// AniListBannerURL is the wide banner from AniList (if any), used with Kitsu/TMDB to pick a Stremio hero image.
	AniListBannerURL string `json:"al_banner,omitempty"`
	// StremioHeroBackgroundURL is the chosen wide backdrop for Stremio meta.background (1280×720-oriented pick incl. TMDB).
	StremioHeroBackgroundURL string `json:"hero_bg,omitempty"`
	Description       string   `json:"description,omitempty"`
	Genres            []string       `json:"genres,omitempty"`
	StartYear         int            `json:"start_year,omitempty"`
	EpisodeLengthMin  int            `json:"ep_min,omitempty"`
	TrailerYouTubeID  string         `json:"trailer_yt,omitempty"`
	TitlePreferred    string         `json:"title_pref,omitempty"`    // romaji / English / ASCII userPreferred (Stremio catalog listing)
	TitleNative       string         `json:"title_native,omitempty"`  // Japanese (optional; meta detail)
	MalID             int            `json:"mal_id,omitempty"`        // MyAnimeList id (AniList idMal / Jikan)
	ImdbID            string         `json:"imdb,omitempty"`          // tt… when Jikan/MAL lists IMDb (TMDB find)
	AniListSearchVer  int            `json:"al_search_ver,omitempty"` // bump forces refetch after search logic changes
	EpisodeTitleByNum map[int]string `json:"ep_titles"`
	// NextAiring* from AniList nextAiringEpisode (Stremio Calendar). NextAiringFromAniList=true means the last
	// AniList fetch set these values (including zeros when nothing is scheduled); Jikan/Kitsu merges must not overwrite.
	NextAiringUnix        int64 `json:"next_air_unix,omitempty"`    // Unix seconds; 0 = none
	NextAiringEpisode     int   `json:"next_air_ep,omitempty"`      // next broadcast episode number
	NextAiringFromAniList bool  `json:"next_air_from_al,omitempty"` // merge: only update airing when true on add
}

// AniListNeedsRefetch is true when we should call AniList again (missing data or legacy poster-only row).
func AniListNeedsRefetch(en AniListSeriesEnrichment) bool {
	if en.AniListSearchVer < AniListSearcherVersion {
		return true
	}
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
	// Refresh schedule after the announced air time so Calendar gets the next slot.
	if en.NextAiringFromAniList && en.NextAiringUnix > 0 && time.Now().Unix() >= en.NextAiringUnix+3600 {
		return true
	}
	return false
}

// EnrichmentCouldUseJikan is true when key Stremio fields are still empty after AniList (Jikan may fill them).
func EnrichmentCouldUseJikan(en AniListSeriesEnrichment) bool {
	if strings.TrimSpace(en.Description) == "" {
		return true
	}
	if strings.TrimSpace(en.PosterURL) == "" {
		return true
	}
	if len(en.Genres) == 0 {
		return true
	}
	if en.StartYear == 0 {
		return true
	}
	// AniList streamingEpisodes often empty; MAL episode list has titles per episode number.
	if len(en.EpisodeTitleByNum) == 0 {
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
	if strings.TrimSpace(out.AniListBannerURL) == "" {
		out.AniListBannerURL = strings.TrimSpace(add.AniListBannerURL)
	}
	if strings.TrimSpace(out.StremioHeroBackgroundURL) == "" {
		out.StremioHeroBackgroundURL = strings.TrimSpace(add.StremioHeroBackgroundURL)
	}
	if out.MalID == 0 && add.MalID > 0 {
		out.MalID = add.MalID
	}
	if strings.TrimSpace(out.ImdbID) == "" {
		out.ImdbID = strings.TrimSpace(add.ImdbID)
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
	if strings.TrimSpace(out.TitleNative) == "" {
		out.TitleNative = strings.TrimSpace(add.TitleNative)
	}
	if add.AniListSearchVer > out.AniListSearchVer {
		out.AniListSearchVer = add.AniListSearchVer
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
	if add.NextAiringFromAniList {
		out.NextAiringUnix = add.NextAiringUnix
		out.NextAiringEpisode = add.NextAiringEpisode
		out.NextAiringFromAniList = true
	}
	return out
}
