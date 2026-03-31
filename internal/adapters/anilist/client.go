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

// SearchResult is the best match for a free-text title.
type SearchResult struct {
	PosterURL string
	Title     string
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
	CoverImage  gqlCoverImage  `json:"coverImage"`
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

const searchQuery = `query ($search: String) {
  Page(page: 1, perPage: 1) {
    media(search: $search, type: ANIME, sort: SEARCH_MATCH, isAdult: false) {
      title { userPreferred romaji english }
      coverImage { extraLarge large }
    }
  }
}`

// SearchAnimePoster returns the cover image URL for the best search match.
func (c *Client) SearchAnimePoster(ctx context.Context, title string) (SearchResult, error) {
	var zero SearchResult
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
		Query: searchQuery,
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
	poster := strings.TrimSpace(m.CoverImage.ExtraLarge)
	if poster == "" {
		poster = strings.TrimSpace(m.CoverImage.Large)
	}
	if poster == "" {
		return zero, errors.New("anilist: no cover image")
	}
	outTitle := strings.TrimSpace(m.Title.UserPreferred)
	if outTitle == "" {
		outTitle = strings.TrimSpace(m.Title.Romaji)
	}
	if outTitle == "" {
		outTitle = strings.TrimSpace(m.Title.English)
	}
	return SearchResult{PosterURL: poster, Title: outTitle}, nil
}
