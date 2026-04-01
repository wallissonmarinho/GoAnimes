package jikan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const defaultBase = "https://api.jikan.moe/v4"

// Client queries Jikan (MyAnimeList) HTTP API v4.
type Client struct {
	getter *httpclient.Getter
	base   string
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

// NewClient returns a Jikan client. getter must be non-nil.
func NewClient(g *httpclient.Getter, opts ...Option) *Client {
	if g == nil {
		return nil
	}
	c := &Client{getter: g, base: defaultBase}
	for _, o := range opts {
		o(c)
	}
	return c
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
		body, err := c.getter.GetBytes(u)
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
		time.Sleep(400 * time.Millisecond)
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
	q := strings.TrimSpace(title)
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
	body, err := c.getter.GetBytes(searchURL)
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
	body2, err := c.getter.GetBytes(detailURL)
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
	desc := anilist.NormalizeDescription(a.Synopsis)
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
		Genres:           genres,
		StartYear:        year,
		EpisodeLengthMin: epMin,
		TrailerYouTubeID: trailer,
		TitlePreferred:   title,
	}
	return out
}
