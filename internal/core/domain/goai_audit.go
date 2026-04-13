package domain

import "time"

// GoaiAuditPromptVersion must match GoAI domain.AuditPromptVersion when interpreting cached JSON.
const GoaiAuditPromptVersion = 3

// GoaiSeriesAuditRequest mirrors GoAI POST /v1/audit/series (no import of GoAI module).
type GoaiSeriesAuditRequest struct {
	SeriesName           string `json:"series_name,omitempty"`
	TorrentTitle         string `json:"torrent_title,omitempty"`
	TorrentLink          string `json:"torrent_link,omitempty"`
	SeriesID             string `json:"series_id,omitempty"`
	MalID                int    `json:"mal_id,omitempty"`
	ImdbID               string `json:"imdb_id,omitempty"`
	Year                 int    `json:"year,omitempty"`
	TitlePreferred       string `json:"title_preferred,omitempty"`
	TitleNative          string `json:"title_native,omitempty"`
	ExistingTVDBSeriesID int    `json:"existing_tvdb_series_id,omitempty"`
	ExistingAniDBAID     int    `json:"existing_anidb_aid,omitempty"`
	ExistingAniListID    int    `json:"existing_anilist_id,omitempty"`
	ExistingTMDBTVID     int    `json:"existing_tmdb_tv_id,omitempty"`
}

// GoaiSeriesAuditResponse mirrors GoAI series audit JSON output.
type GoaiSeriesAuditResponse struct {
	TheTVDBSeriesID  int     `json:"thetvdb_series_id"`
	MalID            int     `json:"mal_id"`
	AniDBAID         int     `json:"anidb_aid"`
	AniListID        int     `json:"anilist_id"`
	TMDBTVID         int     `json:"tmdb_tv_id"`
	ReleaseSeason    int     `json:"release_season"`
	ReleaseEpisode   int     `json:"release_episode"`
	ReleaseIsSpecial bool    `json:"release_is_special"`
	Confidence       float64 `json:"confidence"`
	Notes            string  `json:"notes,omitempty"`
	TheTVDBName      string  `json:"thetvdb_name,omitempty"`
	TheTVDBSlug      string  `json:"thetvdb_slug,omitempty"`
	TheTVDBSeriesURL string  `json:"thetvdb_series_url,omitempty"`
}

// GoaiReleaseAuditRequest mirrors GoAI POST /v1/audit/release.
type GoaiReleaseAuditRequest struct {
	TorrentTitle   string `json:"torrent_title"`
	SeriesName     string `json:"series_name,omitempty"`
	SeriesID       string `json:"series_id,omitempty"`
	CurrentSeason  int    `json:"current_season,omitempty"`
	CurrentEpisode int    `json:"current_episode,omitempty"`
	IsSpecial      bool   `json:"is_special,omitempty"`
}

// GoaiReleaseAuditResponse mirrors GoAI release audit JSON output.
type GoaiReleaseAuditResponse struct {
	Season     int     `json:"season"`
	Episode    int     `json:"episode"`
	IsSpecial  bool    `json:"is_special"`
	Confidence float64 `json:"confidence"`
	Notes      string  `json:"notes,omitempty"`
}

// GoaiSeriesAuditRecord is persisted goai_series_audit row.
type GoaiSeriesAuditRecord struct {
	SeriesID           string
	AuditedAt          time.Time
	PromptVersion      int
	ResponseJSON       string
	NeedsReaudit       bool
	ReauditRequestedAt *time.Time
	Response           *GoaiSeriesAuditResponse // lazy from ResponseJSON when needed
}

// GoaiSeriesAuditListItem is one row for admin list (includes series display name).
type GoaiSeriesAuditListItem struct {
	SeriesID           string     `json:"series_id"`
	SeriesName         string     `json:"series_name"`
	AuditedAt          time.Time  `json:"audited_at"`
	PromptVersion      int        `json:"prompt_version"`
	NeedsReaudit       bool       `json:"needs_reaudit"`
	ReauditRequestedAt *time.Time `json:"reaudit_requested_at,omitempty"`
}

// GoaiReleaseKey identifies a logical episode for dedupe (catalog_item / goai_release_audit).
type GoaiReleaseKey struct {
	SeriesID  string
	Season    int
	Episode   int
	IsSpecial bool
}

// GoaiAuditListParams carries pagination inputs for admin list endpoint.
type GoaiAuditListParams struct {
	Limit  int
	Offset int
}

// GoaiSeriesAuditPage is a paginated response model for admin listing.
type GoaiSeriesAuditPage struct {
	Items  []GoaiSeriesAuditListItem `json:"items"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
	Total  int                       `json:"total"`
}

// GoaiSeriesReauditRequest is the service-level input for re-audit request.
type GoaiSeriesReauditRequest struct {
	SeriesID string
	Scope    string
}

// GoaiSeriesReauditResult is the service-level outcome for re-audit request.
type GoaiSeriesReauditResult struct {
	SeriesID             string `json:"series_id"`
	ClearedReleaseAudits bool   `json:"cleared_release_audits"`
}
