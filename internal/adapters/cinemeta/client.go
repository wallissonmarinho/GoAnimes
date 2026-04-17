package cinemeta

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
)

type Client struct {
	g    *httpclient.Getter
	base string
}

type SearchResponse struct {
	Metas []SearchMeta `json:"metas"`
}

type SearchMeta struct {
	ID      string `json:"id"`
	IMDBID  string `json:"imdb_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Poster  string `json:"poster"`
	Release string `json:"releaseInfo"`
}

type MetaResponse struct {
	Meta SeriesMeta `json:"meta"`
}

type SeriesMeta struct {
	ID          string      `json:"id"`
	IMDBID      string      `json:"imdb_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Poster      string      `json:"poster"`
	Background  string      `json:"background"`
	ReleaseInfo string      `json:"releaseInfo"`
	Released    string      `json:"released"`
	Year        string      `json:"year"`
	Status      string      `json:"status"`
	Genre       []string    `json:"genre"`
	TVDBID      int         `json:"tvdb_id"`
	MovieDBID   int         `json:"moviedb_id"`
	Videos      []MetaVideo `json:"videos"`
}

type MetaVideo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Season     int    `json:"season"`
	Number     int    `json:"number"`
	Episode    int    `json:"episode"`
	Thumbnail  string `json:"thumbnail"`
	Released   string `json:"released,omitempty"`
	FirstAired string `json:"firstAired,omitempty"`
}

func NewClient(g *httpclient.Getter, baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "https://v3-cinemeta.strem.io"
	}
	return &Client{
		g:    g,
		base: strings.TrimRight(baseURL, "/"),
	}
}

func (c *Client) SearchSeries(ctx context.Context, query string) ([]SearchMeta, error) {
	if c == nil || c.g == nil {
		return nil, nil
	}
	u := c.base + "/catalog/series/top/search=" + url.QueryEscape(strings.TrimSpace(query)) + ".json"
	b, err := c.g.GetBytesGETRetry(ctx, u, 2, time.Second)
	if err != nil {
		return nil, err
	}
	var out SearchResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out.Metas, nil
}

func (c *Client) GetSeriesMeta(ctx context.Context, id string) (*SeriesMeta, error) {
	if c == nil || c.g == nil {
		return nil, nil
	}
	u := c.base + "/meta/series/" + url.PathEscape(strings.TrimSpace(id)) + ".json"
	b, err := c.g.GetBytesGETRetry(ctx, u, 2, time.Second)
	if err != nil {
		return nil, err
	}
	var out MetaResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out.Meta, nil
}
