package jikan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const defaultBase = "https://api.jikan.moe/v4"

// defaultMinRequestInterval spaces every Jikan HTTP call (~1.3 r/s) to reduce 429 during sync
// (each enrichment does search + detail + episode pages).
const defaultMinRequestInterval = 750 * time.Millisecond

// Client queries Jikan (MyAnimeList) HTTP API v4.
type Client struct {
	getter      *httpclient.Getter
	base        string
	minInterval time.Duration
	mu          sync.Mutex
	lastEnd     time.Time
}

// Option configures Client.
type Option func(*Client)

// WithBaseURL sets the API root including /v4 (for tests).
func WithBaseURL(base string) Option {
	return func(c *Client) {
		if strings.TrimSpace(base) != "" {
			c.base = strings.TrimSuffix(strings.TrimSpace(base), "/")
		}
	}
}

// WithMinRequestInterval sets the minimum delay between outbound Jikan requests (0 = no pacing; tests).
func WithMinRequestInterval(d time.Duration) Option {
	return func(c *Client) {
		c.minInterval = d
	}
}

// NewClient returns a Jikan client. getter must be non-nil.
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

// getBytesJikan paces requests, retries a few times on HTTP 429.
func (c *Client) getBytesJikan(ctx context.Context, urlStr string) ([]byte, error) {
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
		b, err := c.getter.GetBytes(urlStr)
		c.noteReqDone()
		if err == nil {
			return b, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

type animeSearchResp struct {
	Data []struct {
		MalID int `json:"mal_id"`
	} `json:"data"`
}

type animeByIDResp struct {
	Data jikanAnime `json:"data"`
}

type jikanAnime struct {
	Title        string  `json:"title"`
	TitleEnglish *string `json:"title_english"`
	Synopsis     string  `json:"synopsis"`
	Year         *int    `json:"year"`
	Duration     string  `json:"duration"`
	Genres       []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Images  jikanImages `json:"images"`
	Trailer struct {
		YoutubeID *string `json:"youtube_id"`
	} `json:"trailer"`
}

type jikanImages struct {
	Jpg struct {
		LargeImageURL string `json:"large_image_url"`
	} `json:"jpg"`
}

var (
	durationMinRe   = regexp.MustCompile(`(?i)(\d+)\s*min`)
	malEpisodeNumRe = regexp.MustCompile(`(?i)/episode/(\d+)`)
)

const maxEpisodeListPages = 30 // 100 eps/page → up to 3000; avoids hammering Jikan on long runners

type episodesListResp struct {
	Pagination struct {
		HasNextPage bool `json:"has_next_page"`
	} `json:"pagination"`
	Data []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"data"`
}

func (c *Client) fetchEpisodeTitles(ctx context.Context, malID int) (map[int]string, error) {
	out := make(map[int]string)
	for page := 1; page <= maxEpisodeListPages; page++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		u := fmt.Sprintf("%s/anime/%d/episodes?page=%d", c.base, malID, page)
		body, err := c.getBytesJikan(ctx, u)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		var resp episodesListResp
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, ep := range resp.Data {
			m := malEpisodeNumRe.FindStringSubmatch(ep.URL)
			if len(m) < 2 {
				continue
			}
			n, err := strconv.Atoi(m[1])
			if err != nil || n < 1 {
				continue
			}
			t := strings.TrimSpace(ep.Title)
			if t == "" {
				continue
			}
			if _, ok := out[n]; !ok {
				out[n] = t
			}
		}
		if !resp.Pagination.HasNextPage {
			break
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// SearchAnimeEnrichment returns metadata for the best search match (fills gaps vs AniList).
func (c *Client) SearchAnimeEnrichment(ctx context.Context, title string) (domain.AniListSeriesEnrichment, error) {
	var zero domain.AniListSeriesEnrichment
	if c == nil || c.getter == nil {
		return zero, errors.New("jikan: nil client")
	}
	q := domain.NormalizeExternalAnimeSearchQuery(title)
	if q == "" {
		return zero, errors.New("jikan: empty title")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	searchURL := c.base + "/anime?" + url.Values{
		"q":     {q},
		"limit": {"1"},
		"sfw":   {"true"},
	}.Encode()
	body, err := c.getBytesJikan(ctx, searchURL)
	if err != nil {
		return zero, err
	}
	var sresp animeSearchResp
	if err := json.Unmarshal(body, &sresp); err != nil {
		return zero, err
	}
	if len(sresp.Data) == 0 || sresp.Data[0].MalID <= 0 {
		return zero, errors.New("jikan: no results")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	detailURL := c.base + "/anime/" + strconv.Itoa(sresp.Data[0].MalID)
	body2, err := c.getBytesJikan(ctx, detailURL)
	if err != nil {
		return zero, err
	}
	var dresp animeByIDResp
	if err := json.Unmarshal(body2, &dresp); err != nil {
		return zero, err
	}
	en := jikanAnimeToEnrichment(dresp.Data)
	malID := sresp.Data[0].MalID
	if eps, err := c.fetchEpisodeTitles(ctx, malID); err == nil && len(eps) > 0 {
		en.EpisodeTitleByNum = eps
	}
	return en, nil
}

func jikanAnimeToEnrichment(a jikanAnime) domain.AniListSeriesEnrichment {
	title := strings.TrimSpace(a.Title)
	if a.TitleEnglish != nil {
		if t := strings.TrimSpace(*a.TitleEnglish); t != "" {
			title = t
		}
	}
	var genres []string
	for _, g := range a.Genres {
		if n := strings.TrimSpace(g.Name); n != "" {
			genres = append(genres, n)
		}
	}
	poster := strings.TrimSpace(a.Images.Jpg.LargeImageURL)
	desc := domain.LocalizeAniListDescriptionPTBR(anilist.NormalizeDescription(a.Synopsis))
	var year int
	if a.Year != nil {
		year = *a.Year
	}
	epMin := 0
	if m := durationMinRe.FindStringSubmatch(a.Duration); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			epMin = n
		}
	}
	var trailer string
	if a.Trailer.YoutubeID != nil {
		trailer = strings.TrimSpace(*a.Trailer.YoutubeID)
	}
	out := domain.AniListSeriesEnrichment{
		PosterURL:        poster,
		Description:      desc,
		Genres:           domain.TranslateAnimeGenresToPTBR(genres),
		StartYear:        year,
		EpisodeLengthMin: epMin,
		TrailerYouTubeID: trailer,
		TitlePreferred:   title,
	}
	return out
}
