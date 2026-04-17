package domain

import (
	"strconv"
	"strings"
)

// SeasonEpisodeScheduleKey is the lookup key for EpisodeReleasedBySeasonEpisode (Cinemeta season + in-season number).
func SeasonEpisodeScheduleKey(season, episodeInSeason int) string {
	return strconv.Itoa(season) + ":" + strconv.Itoa(episodeInSeason)
}

// SeriesEnrichment is cached Stremio-facing metadata per series id.
type SeriesEnrichment struct {
	PosterURL     string `json:"poster,omitempty"`
	BackgroundURL string `json:"background,omitempty"`
	// AniListBannerURL keeps the existing serialized field name for compatibility.
	AniListBannerURL string `json:"al_banner,omitempty"`
	// StremioHeroBackgroundURL is the chosen wide backdrop for Stremio meta.background (1280×720-oriented pick incl. TMDB).
	StremioHeroBackgroundURL string   `json:"hero_bg,omitempty"`
	Description              string   `json:"description,omitempty"`
	Genres                   []string `json:"genres,omitempty"`
	StartYear                int      `json:"start_year,omitempty"`
	EpisodeLengthMin         int      `json:"ep_min,omitempty"`
	TrailerYouTubeID         string   `json:"trailer_yt,omitempty"`
	TitlePreferred           string   `json:"title_pref,omitempty"`   // romaji / English / ASCII userPreferred (Stremio catalog listing)
	TitleNative              string   `json:"title_native,omitempty"` // Japanese (optional; meta detail)
	MalID                    int      `json:"mal_id,omitempty"`       // MyAnimeList id
	ImdbID                   string   `json:"imdb,omitempty"`         // tt… id
	// TvdbSeriesID is TheTVDB v4 series id (remote IMDb search + episode/artwork APIs).
	TvdbSeriesID     int `json:"tvdb_id,omitempty"`
	AniListSearchVer int `json:"al_search_ver,omitempty"` // bump forces refetch after search logic changes
	// SeriesStatus, SeriesReleasedISO, SeriesYearLabel mirror Cinemeta/Stremio meta (when available).
	SeriesStatus      string         `json:"series_status,omitempty"`
	SeriesReleasedISO string         `json:"series_released,omitempty"`
	SeriesYearLabel   string         `json:"series_year,omitempty"`
	EpisodeTitleByNum map[int]string `json:"ep_titles"`
	// EpisodeThumbnailByNum is Stremio video thumbnail per episode number.
	EpisodeThumbnailByNum map[int]string `json:"ep_thumbs,omitempty"`
	// EpisodeReleasedBySeasonEpisode maps "season:episodeInSeason" (e.g. "1:5") to an ISO 8601 instant from Cinemeta (Stremio "upcoming" badge uses future released).
	EpisodeReleasedBySeasonEpisode map[string]string `json:"ep_released_se,omitempty"`
	// NextAiring* keeps scheduled calendar metadata. NextAiringFromAniList keeps the serialized field name.
	NextAiringUnix        int64 `json:"next_air_unix,omitempty"`    // Unix seconds; 0 = none
	NextAiringEpisode     int   `json:"next_air_ep,omitempty"`      // next broadcast episode number
	NextAiringFromAniList bool  `json:"next_air_from_al,omitempty"` // merge: only update airing when true on add
}

// episodeTitleMapHasAnyNonEmpty is true if m has at least one non-blank title.
func episodeTitleMapHasAnyNonEmpty(m map[int]string) bool {
	if m == nil {
		return false
	}
	for _, v := range m {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// EnrichmentHasAnyEpisodeTitle is true when at least one episode number has a non-blank title (after trim).
func EnrichmentHasAnyEpisodeTitle(en SeriesEnrichment) bool {
	return episodeTitleMapHasAnyNonEmpty(en.EpisodeTitleByNum)
}

// MergeSeriesEnrichment fills empty fields in stored with values from add (e.g. DB row + lazy Cinemeta fetch).
func MergeSeriesEnrichment(stored, add SeriesEnrichment) SeriesEnrichment {
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
	if out.TvdbSeriesID == 0 && add.TvdbSeriesID > 0 {
		out.TvdbSeriesID = add.TvdbSeriesID
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
	if strings.TrimSpace(out.SeriesStatus) == "" {
		out.SeriesStatus = strings.TrimSpace(add.SeriesStatus)
	}
	if strings.TrimSpace(out.SeriesReleasedISO) == "" {
		out.SeriesReleasedISO = strings.TrimSpace(add.SeriesReleasedISO)
	}
	if strings.TrimSpace(out.SeriesYearLabel) == "" {
		out.SeriesYearLabel = strings.TrimSpace(add.SeriesYearLabel)
	}
	if add.AniListSearchVer > out.AniListSearchVer {
		out.AniListSearchVer = add.AniListSearchVer
	}
	if add.EpisodeTitleByNum != nil {
		if out.EpisodeTitleByNum == nil {
			out.EpisodeTitleByNum = make(map[int]string)
		}
		for k, v := range add.EpisodeTitleByNum {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			cur, ok := out.EpisodeTitleByNum[k]
			if !ok || strings.TrimSpace(cur) == "" {
				out.EpisodeTitleByNum[k] = v
			}
		}
	}
	if add.EpisodeThumbnailByNum != nil {
		if out.EpisodeThumbnailByNum == nil {
			out.EpisodeThumbnailByNum = make(map[int]string)
		}
		for k, v := range add.EpisodeThumbnailByNum {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			cur, ok := out.EpisodeThumbnailByNum[k]
			if !ok || strings.TrimSpace(cur) == "" {
				out.EpisodeThumbnailByNum[k] = v
			}
		}
	}
	if add.EpisodeReleasedBySeasonEpisode != nil {
		if out.EpisodeReleasedBySeasonEpisode == nil {
			out.EpisodeReleasedBySeasonEpisode = make(map[string]string)
		}
		for k, v := range add.EpisodeReleasedBySeasonEpisode {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			out.EpisodeReleasedBySeasonEpisode[k] = v
		}
	}
	if add.NextAiringFromAniList {
		out.NextAiringUnix = add.NextAiringUnix
		out.NextAiringEpisode = add.NextAiringEpisode
		out.NextAiringFromAniList = true
	}
	return out
}
