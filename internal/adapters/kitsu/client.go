package kitsu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const defaultBase = "https://kitsu.io/api/edge"

// defaultMinRequestInterval spaces Kitsu JSON:API GETs to stay polite on shared infrastructure.
const defaultMinRequestInterval = 400 * time.Millisecond

// Client queries Kitsu JSON:API (no API key for public reads).
type Client struct {
	getter      *httpclient.Getter
	base        string
	minInterval time.Duration
	mu          sync.Mutex
	lastEnd     time.Time
}

// Option configures Client.
type Option func(*Client)

// WithBaseURL sets the API root (for tests).
func WithBaseURL(base string) Option {
	return func(c *Client) {
		if strings.TrimSpace(base) != "" {
			c.base = strings.TrimSuffix(strings.TrimSpace(base), "/")
		}
	}
}

// WithMinRequestInterval sets the minimum delay between outbound requests (0 = no pacing; tests).
func WithMinRequestInterval(d time.Duration) Option {
	return func(c *Client) {
		c.minInterval = d
	}
}

// NewClient returns a Kitsu client. getter must be non-nil.
func NewClient(g *httpclient.Getter, opts ...Option) *Client {
	if g == nil {
		return nil
	}
	c := &Client{getter: g, base: defaultBase, minInterval: defaultMinRequestInterval}
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

func (c *Client) getJSONAPI(ctx context.Context, urlStr string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			var he *httpclient.HTTPStatusError
			if !errors.As(lastErr, &he) || he.StatusCode != http.StatusTooManyRequests {
				return nil, lastErr
			}
			back := time.Duration(500+attempt*350) * time.Millisecond
			if back > 5*time.Second {
				back = 5 * time.Second
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
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.api+json")
		if c.getter.UserAgent != "" {
			req.Header.Set("User-Agent", c.getter.UserAgent)
		}
		resp, err := c.getter.Client.Do(req)
		c.noteReqDone()
		if err != nil {
			lastErr = err
			continue
		}
		status := resp.StatusCode
		r := io.LimitReader(resp.Body, c.getter.MaxBodyBytes+1)
		b, rerr := io.ReadAll(r)
		_ = resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if int64(len(b)) > c.getter.MaxBodyBytes {
			lastErr = fmt.Errorf("body exceeds max bytes")
			continue
		}
		if status < 200 || status >= 300 {
			lastErr = &httpclient.HTTPStatusError{StatusCode: status}
			continue
		}
		return b, nil
	}
	return nil, lastErr
}

type jsonapiResource struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Attributes    json.RawMessage `json:"attributes"`
	Relationships json.RawMessage `json:"relationships"`
}

type animeSearchResp struct {
	Data     []jsonapiResource `json:"data"`
	Included []jsonapiResource `json:"included"`
}

// kitsuImageURLs matches Kitsu posterImage/coverImage URL fields. Nested keys (e.g. meta.dimensions) are ignored by encoding/json.
type kitsuImageURLs struct {
	Tiny     string `json:"tiny"`
	Small    string `json:"small"`
	Medium   string `json:"medium"`
	Large    string `json:"large"`
	Original string `json:"original"`
}

type animeAttrs struct {
	Synopsis       string            `json:"synopsis"`
	Titles         map[string]string `json:"titles"`
	CanonicalTitle string            `json:"canonicalTitle"`
	StartDate      string            `json:"startDate"`
	EpisodeLength  *int              `json:"episodeLength"`
	PosterImage    kitsuImageURLs    `json:"posterImage"`
	CoverImage     kitsuImageURLs    `json:"coverImage"`
}

type categoryAttrs struct {
	Title string `json:"title"`
}

type relDataEntry struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// SearchAnimeEnrichment returns metadata for the best text search match (fills gaps vs AniList/Jikan).
func (c *Client) SearchAnimeEnrichment(ctx context.Context, title string) (domain.AniListSeriesEnrichment, error) {
	var zero domain.AniListSeriesEnrichment
	if c == nil || c.getter == nil || c.getter.Client == nil {
		return zero, errors.New("kitsu: nil client")
	}
	q := domain.NormalizeExternalAnimeSearchQuery(title)
	if q == "" {
		return zero, errors.New("kitsu: empty title")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	u := c.base + "/anime?" + url.Values{
		"filter[text]":  {q},
		"page[limit]":   {"1"},
		"include":       {"categories"},
	}.Encode()
	body, err := c.getJSONAPI(ctx, u)
	if err != nil {
		return zero, err
	}
	var resp animeSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return zero, err
	}
	if len(resp.Data) == 0 || resp.Data[0].Type != "anime" {
		return zero, errors.New("kitsu: no results")
	}
	var attrs animeAttrs
	if err := json.Unmarshal(resp.Data[0].Attributes, &attrs); err != nil {
		return zero, err
	}
	catTitles := categoryTitlesFromIncluded(resp.Data[0].Relationships, resp.Included)
	kid := strings.TrimSpace(resp.Data[0].ID)
	return kitsuAnimeToEnrichment(attrs, catTitles, kid), nil
}

// SearchAnimeID returns the Kitsu anime id for the best text match (limit 1), or error.
func (c *Client) SearchAnimeID(ctx context.Context, title string) (string, error) {
	if c == nil || c.getter == nil || c.getter.Client == nil {
		return "", errors.New("kitsu: nil client")
	}
	q := domain.NormalizeExternalAnimeSearchQuery(title)
	if q == "" {
		return "", errors.New("kitsu: empty title")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	u := c.base + "/anime?" + url.Values{
		"filter[text]": {q},
		"page[limit]":  {"1"},
	}.Encode()
	body, err := c.getJSONAPI(ctx, u)
	if err != nil {
		return "", err
	}
	var resp animeSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 || resp.Data[0].Type != "anime" {
		return "", errors.New("kitsu: no results")
	}
	return strings.TrimSpace(resp.Data[0].ID), nil
}

type episodeAttrs struct {
	Number         int               `json:"number"`
	SeasonNumber   int               `json:"seasonNumber"`
	CanonicalTitle *string           `json:"canonicalTitle"`
	Titles         map[string]string `json:"titles"`
	Thumbnail      *struct {
		Original string `json:"original"`
	} `json:"thumbnail"`
}

func kitsuEpisodePickTitle(a episodeAttrs) string {
	if a.CanonicalTitle != nil {
		if t := strings.TrimSpace(*a.CanonicalTitle); t != "" {
			return t
		}
	}
	if a.Titles != nil {
		for _, k := range []string{"en_us", "en_jp", "en", "ja_jp"} {
			if t := strings.TrimSpace(a.Titles[k]); t != "" {
				return t
			}
		}
		for _, t := range a.Titles {
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		}
	}
	return ""
}

// FetchEpisodeMaps loads episode titles and thumbnails from Kitsu /episodes (paginated) for a media id.
func (c *Client) FetchEpisodeMaps(ctx context.Context, kitsuAnimeID string) (map[int]string, map[int]string, error) {
	titles := make(map[int]string)
	thumbs := make(map[int]string)
	kitsuAnimeID = strings.TrimSpace(kitsuAnimeID)
	if c == nil || c.getter == nil {
		return nil, nil, errors.New("kitsu: nil client")
	}
	if kitsuAnimeID == "" {
		return nil, nil, errors.New("kitsu: empty anime id")
	}
	const limit = 100
	offset := 0
	total := -1
	for page := 0; page < 80; page++ {
		if err := ctx.Err(); err != nil {
			return titles, thumbs, err
		}
		u := c.base + "/episodes?" + url.Values{
			"filter[mediaId]": {kitsuAnimeID},
			"sort":            {"number"},
			"page[limit]":     {strconv.Itoa(limit)},
			"page[offset]":    {strconv.Itoa(offset)},
		}.Encode()
		body, err := c.getJSONAPI(ctx, u)
		if err != nil {
			return nil, nil, err
		}
		var resp struct {
			Data []jsonapiResource `json:"data"`
			Meta struct {
				Count int `json:"count"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, nil, err
		}
		if page == 0 {
			total = resp.Meta.Count
		}
		for _, row := range resp.Data {
			if row.Type != "episodes" {
				continue
			}
			var attrs episodeAttrs
			if err := json.Unmarshal(row.Attributes, &attrs); err != nil {
				continue
			}
			n := attrs.Number
			if n < 1 {
				continue
			}
			if t := kitsuEpisodePickTitle(attrs); t != "" {
				if _, ok := titles[n]; !ok {
					titles[n] = t
				}
			}
			if attrs.Thumbnail != nil {
				if u := strings.TrimSpace(attrs.Thumbnail.Original); u != "" {
					if _, ok := thumbs[n]; !ok {
						thumbs[n] = u
					}
				}
			}
		}
		nGot := len(resp.Data)
		offset += nGot
		if nGot == 0 {
			break
		}
		if total > 0 && offset >= total {
			break
		}
		if total <= 0 && nGot < limit {
			break
		}
	}
	return titles, thumbs, nil
}

func categoryTitlesFromIncluded(relationships json.RawMessage, included []jsonapiResource) []string {
	if len(relationships) == 0 {
		return nil
	}
	var wrap struct {
		Categories *struct {
			Data json.RawMessage `json:"data"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(relationships, &wrap); err != nil || wrap.Categories == nil {
		return nil
	}
	var single relDataEntry
	if err := json.Unmarshal(wrap.Categories.Data, &single); err == nil && single.ID != "" {
		return titlesForCategoryIDs([]relDataEntry{single}, included)
	}
	var many []relDataEntry
	if err := json.Unmarshal(wrap.Categories.Data, &many); err != nil {
		return nil
	}
	return titlesForCategoryIDs(many, included)
}

func titlesForCategoryIDs(refs []relDataEntry, included []jsonapiResource) []string {
	byID := make(map[string]jsonapiResource, len(included))
	for _, r := range included {
		byID[r.Type+":"+r.ID] = r
	}
	var out []string
	for _, ref := range refs {
		if ref.Type != "categories" || ref.ID == "" {
			continue
		}
		r, ok := byID["categories:"+ref.ID]
		if !ok {
			continue
		}
		var ca categoryAttrs
		if err := json.Unmarshal(r.Attributes, &ca); err != nil {
			continue
		}
		if t := strings.TrimSpace(ca.Title); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func kitsuAnimeToEnrichment(a animeAttrs, categoryTitles []string, kitsuAnimeID string) domain.AniListSeriesEnrichment {
	syn := strings.TrimSpace(a.Synopsis)
	if syn == "" {
		syn = strings.TrimSpace(a.CanonicalTitle)
	}
	desc := domain.LocalizeAniListDescriptionPTBR(anilist.NormalizeDescription(syn))
	title := pickKitsuTitle(a)
	poster := firstSizedImageURL(a.PosterImage, "original", "large", "medium", "small", "tiny")
	bg := firstSizedImageURL(a.CoverImage, "original", "large", "medium", "small", "tiny")
	year := yearFromStartDate(a.StartDate)
	epMin := 0
	if a.EpisodeLength != nil && *a.EpisodeLength > 0 {
		epMin = *a.EpisodeLength
	}
	genres := domain.TranslateAnimeGenresToPTBR(categoryTitles)
	return domain.AniListSeriesEnrichment{
		PosterURL:        poster,
		BackgroundURL:    bg,
		Description:      desc,
		Genres:           genres,
		StartYear:        year,
		EpisodeLengthMin: epMin,
		TitlePreferred:   title,
		TitleNative:      strings.TrimSpace(a.Titles["ja_jp"]),
		KitsuAnimeID:     strings.TrimSpace(kitsuAnimeID),
		AniListSearchVer: domain.AniListSearcherVersion,
	}
}

func pickKitsuTitle(a animeAttrs) string {
	if t := strings.TrimSpace(a.Titles["en"]); t != "" {
		return t
	}
	if t := strings.TrimSpace(a.Titles["en_jp"]); t != "" {
		return t
	}
	return strings.TrimSpace(a.CanonicalTitle)
}

func firstSizedImageURL(img kitsuImageURLs, keys ...string) string {
	for _, k := range keys {
		var u string
		switch k {
		case "original":
			u = img.Original
		case "large":
			u = img.Large
		case "medium":
			u = img.Medium
		case "small":
			u = img.Small
		case "tiny":
			u = img.Tiny
		}
		if s := strings.TrimSpace(u); s != "" {
			return s
		}
	}
	return ""
}

func yearFromStartDate(s string) int {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil || y < 1900 {
		return 0
	}
	return y
}
