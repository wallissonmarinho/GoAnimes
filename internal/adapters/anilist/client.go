package anilist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anidb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const defaultEndpoint = "https://graphql.anilist.co"

// defaultAnilistMinInterval spaces GraphQL posts to avoid HTTP 429 on the public API (strict burst limits).
const defaultAnilistMinInterval = 1200 * time.Millisecond

// Client queries AniList GraphQL (no API key required for public reads).
type Client struct {
	getter      *httpclient.Getter
	endpoint    string
	minInterval time.Duration
	mu          sync.Mutex
	lastEnd     time.Time
}

// Option configures Client.
type Option func(*Client)

// WithEndpoint sets the GraphQL URL (tests or self-hosted).
func WithEndpoint(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.endpoint = url
		}
	}
}

// WithMinRequestInterval sets the minimum delay between outbound AniList requests (0 = no pacing; tests).
func WithMinRequestInterval(d time.Duration) Option {
	return func(c *Client) {
		c.minInterval = d
	}
}

func NewClient(g *httpclient.Getter, opts ...Option) *Client {
	if g == nil {
		return nil
	}
	c := &Client{getter: g, endpoint: defaultEndpoint, minInterval: defaultAnilistMinInterval}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) pace(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.minInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	wait := time.Duration(0)
	if !c.lastEnd.IsZero() {
		next := c.lastEnd.Add(c.minInterval)
		if d := time.Until(next); d > 0 {
			wait = d
		}
	}
	c.mu.Unlock()
	if wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil
}

func (c *Client) noteReqDone() {
	if c.minInterval <= 0 {
		return
	}
	c.mu.Lock()
	c.lastEnd = time.Now()
	c.mu.Unlock()
}

func (c *Client) postGQL(ctx context.Context, body []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			var he *httpclient.HTTPStatusError
			if !errors.As(lastErr, &he) || he.StatusCode != http.StatusTooManyRequests {
				return nil, lastErr
			}
			back := time.Duration(800+attempt*500) * time.Millisecond
			if back > 10*time.Second {
				back = 10 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(back):
			}
		}
		if err := c.pace(ctx); err != nil {
			return nil, err
		}
		b, err := c.getter.PostBytes(c.endpoint, "application/json", body)
		c.noteReqDone()
		if err == nil {
			return b, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// MediaDetails is the best search match for a free-text title.
type MediaDetails struct {
	PosterURL         string
	BackgroundURL     string
	Title             string // Latin-friendly (romaji / English); for Stremio catalog
	NativeTitle       string // Japanese native from AniList (meta detail)
	Description       string
	Genres            []string
	StartYear         int
	EpisodeLengthMin  int
	TrailerYouTubeID  string
	EpisodeTitleByNum      map[int]string
	EpisodeThumbnailByNum  map[int]string
	// NextAiring* from AniList nextAiringEpisode (Stremio Calendar); unix 0 if not airing / unknown.
	NextAiringUnix    int64
	NextAiringEpisode int
	MalID             int // AniList idMal → MAL / Jikan
	AniDBAid          int // AniList externalLinks → AniDB HTTP API
}

// ToDomainEnrichment maps API details into the persisted enrichment shape.
func ToDomainEnrichment(d MediaDetails) domain.AniListSeriesEnrichment {
	ep := d.EpisodeTitleByNum
	if ep == nil {
		ep = map[int]string{}
	}
	th := d.EpisodeThumbnailByNum
	if len(th) == 0 {
		th = nil
	}
	desc := domain.LocalizeAniListDescriptionPTBR(d.Description)
	banner := strings.TrimSpace(d.BackgroundURL)
	out := domain.AniListSeriesEnrichment{
		PosterURL:         d.PosterURL,
		BackgroundURL:     banner,
		AniListBannerURL:  banner,
		Description:       desc,
		Genres:            domain.TranslateAnimeGenresToPTBR(append([]string(nil), d.Genres...)),
		StartYear:         d.StartYear,
		EpisodeLengthMin:  d.EpisodeLengthMin,
		TrailerYouTubeID:  d.TrailerYouTubeID,
		TitlePreferred:    d.Title,
		TitleNative:       d.NativeTitle,
		MalID:                 d.MalID,
		AniDBAid:              d.AniDBAid,
		AniListSearchVer:      domain.AniListSearcherVersion,
		EpisodeTitleByNum:     ep,
		EpisodeThumbnailByNum: th,
		NextAiringFromAniList: true,
		NextAiringUnix:       d.NextAiringUnix,
		NextAiringEpisode:    d.NextAiringEpisode,
	}
	return out
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type gqlResponse struct {
	Data   *gqlData   `json:"data"`
	Errors []gqlError `json:"errors"`
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlData struct {
	Page *gqlPage `json:"Page"`
}

type gqlPage struct {
	Media []gqlMedia `json:"media"`
}

type gqlMedia struct {
	IDMal              *int                   `json:"idMal"`
	Title              gqlTitle               `json:"title"`
	CoverImage         gqlCoverImage          `json:"coverImage"`
	BannerImage        *string                `json:"bannerImage"` // AniList: scalar URL (not { large })
	Description        *string                `json:"description"`
	Genres             []string               `json:"genres"`
	SeasonYear         *int                   `json:"seasonYear"`
	StartDate          *gqlFuzzyDate          `json:"startDate"`
	Duration           *int                   `json:"duration"`
	Trailer            *gqlTrailer            `json:"trailer"`
	StreamingEpisodes  []gqlStreamingEpisode  `json:"streamingEpisodes"`
	NextAiringEpisode  *gqlNextAiring         `json:"nextAiringEpisode"`
	ExternalLinks      []gqlExternalLink      `json:"externalLinks"`
}

type gqlExternalLink struct {
	URL    *string `json:"url"`
	Site   string  `json:"site"`
	SiteID *int    `json:"siteId"`
}

type gqlNextAiring struct {
	AiringAt *int `json:"airingAt"` // Unix seconds UTC
	Episode  *int `json:"episode"`
}

type gqlStreamingEpisode struct {
	Title     string  `json:"title"`
	Thumbnail *string `json:"thumbnail"`
}

type gqlFuzzyDate struct {
	Year *int `json:"year"`
}

type gqlTrailer struct {
	ID   string `json:"id"`
	Site string `json:"site"`
}

type gqlTitle struct {
	UserPreferred string `json:"userPreferred"`
	English       string `json:"english"`
	Native        string `json:"native"`
	Romaji        string `json:"romaji"`
}

type gqlCoverImage struct {
	ExtraLarge string `json:"extraLarge"`
	Large      string `json:"large"`
}

const searchMediaQuery = `query ($search: String, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(search: $search, type: ANIME, sort: SEARCH_MATCH, isAdult: false) {
      idMal
      title { userPreferred english native romaji }
      coverImage { extraLarge large }
      bannerImage
      description(asHtml: false)
      genres
      seasonYear
      startDate { year }
      duration
      trailer { id site }
      streamingEpisodes { title thumbnail }
      nextAiringEpisode { airingAt episode }
      externalLinks { url site siteId }
    }
  }
}`

// SearchAnimeMedia returns Stremio-relevant metadata for the best AniList match.
func (c *Client) SearchAnimeMedia(ctx context.Context, title string) (MediaDetails, error) {
	var zero MediaDetails
	if c == nil || c.getter == nil {
		return zero, errors.New("anilist: nil client")
	}
	q := domain.NormalizeExternalAnimeSearchQuery(title)
	if q == "" {
		return zero, errors.New("anilist: empty title")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	candidates := domain.AniListSearchQueryCandidates(q)
	if len(candidates) == 0 {
		return zero, errors.New("anilist: empty title")
	}
	var lastErr error
	for _, search := range candidates {
		body, err := json.Marshal(gqlRequest{
			Query: searchMediaQuery,
			Variables: map[string]any{
				"search":  search,
				"perPage": 15,
			},
		})
		if err != nil {
			return zero, err
		}
		b, err := c.postGQL(ctx, body)
		if err != nil {
			return zero, err
		}
		var resp gqlResponse
		if err := json.Unmarshal(b, &resp); err != nil {
			return zero, err
		}
		if len(resp.Errors) > 0 && resp.Errors[0].Message != "" {
			lastErr = errors.New(resp.Errors[0].Message)
			continue
		}
		if resp.Data == nil || resp.Data.Page == nil || len(resp.Data.Page.Media) == 0 {
			lastErr = errors.New("anilist: no results")
			continue
		}
		m := pickBestSearchMedia(search, resp.Data.Page.Media)
		out := MediaDetails{}
		out.PosterURL = strings.TrimSpace(m.CoverImage.ExtraLarge)
		if out.PosterURL == "" {
			out.PosterURL = strings.TrimSpace(m.CoverImage.Large)
		}
		if m.BannerImage != nil {
			out.BackgroundURL = strings.TrimSpace(*m.BannerImage)
		}
		if m.IDMal != nil && *m.IDMal > 0 {
			out.MalID = *m.IDMal
		}
		out.Title = pickTitleLatin(m.Title)
		out.NativeTitle = strings.TrimSpace(m.Title.Native)
		if m.Description != nil {
			out.Description = NormalizeDescription(*m.Description)
		}
		if len(m.Genres) > 0 {
			out.Genres = append(out.Genres, m.Genres...)
		}
		if m.SeasonYear != nil && *m.SeasonYear > 0 {
			out.StartYear = *m.SeasonYear
		} else if m.StartDate != nil && m.StartDate.Year != nil && *m.StartDate.Year > 0 {
			out.StartYear = *m.StartDate.Year
		}
		if m.Duration != nil && *m.Duration > 0 {
			out.EpisodeLengthMin = *m.Duration
		}
		if m.Trailer != nil {
			site := strings.ToLower(strings.TrimSpace(m.Trailer.Site))
			id := strings.TrimSpace(m.Trailer.ID)
			if id != "" && (site == "youtube" || site == "youtu_be") {
				out.TrailerYouTubeID = id
			}
		}
		if len(m.StreamingEpisodes) > 0 {
			eps := make([]domain.AniListStreamingEpisode, 0, len(m.StreamingEpisodes))
			for _, se := range m.StreamingEpisodes {
				thStr := ""
				if se.Thumbnail != nil {
					thStr = strings.TrimSpace(*se.Thumbnail)
				}
				eps = append(eps, domain.AniListStreamingEpisode{Title: se.Title, Thumbnail: thStr})
			}
			out.EpisodeTitleByNum, out.EpisodeThumbnailByNum = domain.EpisodeStreamingDataFromAniList(eps)
		}
		if m.NextAiringEpisode != nil && m.NextAiringEpisode.AiringAt != nil && *m.NextAiringEpisode.AiringAt > 0 &&
			m.NextAiringEpisode.Episode != nil && *m.NextAiringEpisode.Episode > 0 {
			out.NextAiringUnix = int64(*m.NextAiringEpisode.AiringAt)
			out.NextAiringEpisode = *m.NextAiringEpisode.Episode
		}
		out.AniDBAid = anidbAidFromExternalLinks(m.ExternalLinks)
		if out.PosterURL == "" && out.BackgroundURL == "" && out.Description == "" && out.Title == "" && out.NativeTitle == "" && out.EpisodeLengthMin == 0 && len(out.Genres) == 0 && out.StartYear == 0 && out.TrailerYouTubeID == "" && len(out.EpisodeTitleByNum) == 0 && len(out.EpisodeThumbnailByNum) == 0 && out.NextAiringUnix == 0 && out.AniDBAid == 0 {
			lastErr = errors.New("anilist: empty media payload")
			continue
		}
		return out, nil
	}
	if lastErr != nil {
		return zero, lastErr
	}
	return zero, errors.New("anilist: no results")
}

func mediaTitleHaystack(t gqlTitle) string {
	return strings.ToLower(strings.TrimSpace(t.Romaji + " " + t.English + " " + t.Native + " " + t.UserPreferred))
}

func scoreMediaTitleMatch(t gqlTitle, tokens []string, rawQuery string) int {
	hay := mediaTitleHaystack(t)
	score := 0
	for _, tok := range tokens {
		if tok != "" && strings.Contains(hay, tok) {
			score++
		}
	}
	rq := strings.ToLower(strings.TrimSpace(rawQuery))
	if rq != "" && strings.Contains(hay, rq) {
		if len(tokens) > 0 {
			score += len(tokens) + 2
		} else {
			score++
		}
	}
	return score
}

// pickBestSearchMedia chooses the AniList row that best matches the RSS search string (first hit is often wrong).
func pickBestSearchMedia(search string, list []gqlMedia) gqlMedia {
	if len(list) == 0 {
		return gqlMedia{}
	}
	if len(list) == 1 {
		return list[0]
	}
	tokens := domain.AnimeSearchScoringTokens(search)
	bestI, bestScore := 0, -1
	for i := range list {
		s := scoreMediaTitleMatch(list[i].Title, tokens, search)
		if s > bestScore {
			bestI, bestScore = i, s
		}
	}
	if bestScore <= 0 {
		return list[0]
	}
	return list[bestI]
}

// pickTitleLatin prefers romaji/English for Stremio catalog; avoids Japanese userPreferred when romaji exists.
func anidbAidFromExternalLinks(links []gqlExternalLink) int {
	for _, l := range links {
		site := strings.ToLower(strings.TrimSpace(l.Site))
		if site == "anidb" || strings.Contains(site, "anidb") {
			if l.SiteID != nil && *l.SiteID > 0 {
				return *l.SiteID
			}
			if l.URL != nil {
				if id := anidb.ParseAnidbAidFromURL(*l.URL); id > 0 {
					return id
				}
			}
		}
	}
	return 0
}

func pickTitleLatin(t gqlTitle) string {
	if s := strings.TrimSpace(t.Romaji); s != "" {
		return s
	}
	if s := strings.TrimSpace(t.English); s != "" {
		return s
	}
	if s := strings.TrimSpace(t.UserPreferred); s != "" && !domain.ContainsJapaneseScript(s) {
		return s
	}
	if s := strings.TrimSpace(t.UserPreferred); s != "" {
		return s
	}
	if s := strings.TrimSpace(t.Native); s != "" {
		return s
	}
	return ""
}

// SearchAnimePoster is a thin wrapper (poster + title only) for backward compatibility.
func (c *Client) SearchAnimePoster(ctx context.Context, title string) (MediaDetails, error) {
	return c.SearchAnimeMedia(ctx, title)
}
