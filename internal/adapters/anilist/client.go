package anilist

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
)

const defaultEndpoint = "https://graphql.anilist.co"

// Client queries AniList GraphQL (no API key required for public reads).
type Client struct {
	getter   *httpclient.Getter
	endpoint string
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

func NewClient(g *httpclient.Getter, opts ...Option) *Client {
	if g == nil {
		return nil
	}
	c := &Client{getter: g, endpoint: defaultEndpoint}
	for _, o := range opts {
		o(c)
	}
	return c
}

// MediaDetails is the best search match for a free-text title.
type MediaDetails struct {
	PosterURL        string
	BackgroundURL    string
	Title            string
	Description      string
	Genres           []string
	StartYear        int
	EpisodeLengthMin int
	TrailerYouTubeID string
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
	Title       gqlTitle       `json:"title"`
	CoverImage   gqlCoverImage   `json:"coverImage"`
	BannerImage  *gqlCoverImage  `json:"bannerImage"`
	Description  *string         `json:"description"`
	Genres      []string       `json:"genres"`
	SeasonYear  *int           `json:"seasonYear"`
	StartDate   *gqlFuzzyDate  `json:"startDate"`
	Duration    *int           `json:"duration"`
	Trailer     *gqlTrailer    `json:"trailer"`
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
	Romaji        string `json:"romaji"`
	English       string `json:"english"`
}

type gqlCoverImage struct {
	ExtraLarge string `json:"extraLarge"`
	Large      string `json:"large"`
}

const searchMediaQuery = `query ($search: String) {
  Page(page: 1, perPage: 1) {
    media(search: $search, type: ANIME, sort: SEARCH_MATCH, isAdult: false) {
      title { userPreferred romaji english }
      coverImage { extraLarge large }
      bannerImage { large }
      description(asHtml: false)
      genres
      seasonYear
      startDate { year }
      duration
      trailer { id site }
    }
  }
}`

// SearchAnimeMedia returns Stremio-relevant metadata for the best AniList match.
func (c *Client) SearchAnimeMedia(ctx context.Context, title string) (MediaDetails, error) {
	var zero MediaDetails
	if c == nil || c.getter == nil {
		return zero, errors.New("anilist: nil client")
	}
	q := strings.TrimSpace(title)
	if q == "" {
		return zero, errors.New("anilist: empty title")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	body, err := json.Marshal(gqlRequest{
		Query: searchMediaQuery,
		Variables: map[string]any{
			"search": q,
		},
	})
	if err != nil {
		return zero, err
	}
	b, err := c.getter.PostBytes(c.endpoint, "application/json", body)
	if err != nil {
		return zero, err
	}
	var resp gqlResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return zero, err
	}
	if len(resp.Errors) > 0 && resp.Errors[0].Message != "" {
		return zero, errors.New(resp.Errors[0].Message)
	}
	if resp.Data == nil || resp.Data.Page == nil || len(resp.Data.Page.Media) == 0 {
		return zero, errors.New("anilist: no results")
	}
	m := resp.Data.Page.Media[0]
	out := MediaDetails{}
	out.PosterURL = strings.TrimSpace(m.CoverImage.ExtraLarge)
	if out.PosterURL == "" {
		out.PosterURL = strings.TrimSpace(m.CoverImage.Large)
	}
	if m.BannerImage != nil {
		out.BackgroundURL = strings.TrimSpace(m.BannerImage.Large)
	}
	out.Title = pickTitle(m.Title)
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
	if out.PosterURL == "" && out.BackgroundURL == "" && out.Description == "" && out.Title == "" && out.EpisodeLengthMin == 0 && len(out.Genres) == 0 && out.StartYear == 0 && out.TrailerYouTubeID == "" {
		return zero, errors.New("anilist: empty media payload")
	}
	return out, nil
}

func pickTitle(t gqlTitle) string {
	for _, s := range []string{t.UserPreferred, t.Romaji, t.English} {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// SearchAnimePoster is a thin wrapper (poster + title only) for backward compatibility.
func (c *Client) SearchAnimePoster(ctx context.Context, title string) (MediaDetails, error) {
	return c.SearchAnimeMedia(ctx, title)
}
