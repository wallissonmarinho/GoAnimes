package tmdbapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/ports"
)

type Client struct {
	key     string
	http    *http.Client
	baseURL string
}

func NewClient(apiKey string, timeout time.Duration) *Client {
	return &Client{
		key:     strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: timeout},
		baseURL: "https://api.themoviedb.org/3",
	}
}

func (c *Client) SearchSeries(ctx context.Context, query string) (ports.TMDBSearchResult, bool, error) {
	if c == nil || c.key == "" {
		return ports.TMDBSearchResult{}, false, errors.New("tmdb api key not configured")
	}
	q := url.Values{}
	q.Set("api_key", c.key)
	q.Set("query", strings.TrimSpace(query))
	q.Set("language", "pt-BR")
	endpoint := c.baseURL + "/search/tv?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ports.TMDBSearchResult{}, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ports.TMDBSearchResult{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ports.TMDBSearchResult{}, false, fmt.Errorf("tmdb search failed: %s", resp.Status)
	}
	var payload struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ports.TMDBSearchResult{}, false, err
	}
	if len(payload.Results) == 0 {
		return ports.TMDBSearchResult{}, false, nil
	}
	res := payload.Results[0]
	return ports.TMDBSearchResult{TMDBID: res.ID, Title: res.Name}, true, nil
}

func (c *Client) GetSeasonDetails(ctx context.Context, tmdbID, season int) (ports.TMDBSeasonDetails, error) {
	if c == nil || c.key == "" {
		return ports.TMDBSeasonDetails{}, errors.New("tmdb api key not configured")
	}
	q := url.Values{}
	q.Set("api_key", c.key)
	q.Set("language", "pt-BR")
	endpoint := fmt.Sprintf("%s/tv/%d?%s", c.baseURL, tmdbID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ports.TMDBSeasonDetails{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ports.TMDBSeasonDetails{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ports.TMDBSeasonDetails{}, fmt.Errorf("tmdb details failed: %s", resp.Status)
	}
	var payload struct {
		Name     string `json:"name"`
		Overview string `json:"overview"`
		Poster   string `json:"poster_path"`
		Backdrop string `json:"backdrop_path"`
		Genres   []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Rating float64 `json:"vote_average"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ports.TMDBSeasonDetails{}, err
	}
	genres := make([]string, 0, len(payload.Genres))
	for _, g := range payload.Genres {
		if g.Name != "" {
			genres = append(genres, g.Name)
		}
	}
	poster := payload.Poster
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = "https://image.tmdb.org/t/p/w500" + poster
	}
	return ports.TMDBSeasonDetails{
		Title:        payload.Name,
		Overview:     strings.TrimSpace(payload.Overview),
		PosterPath:   poster,
		BackdropPath: imageURL(payload.Backdrop),
		Genres:       genres,
		Rating:       payload.Rating,
	}, nil
}

func imageURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http") {
		return path
	}
	return "https://image.tmdb.org/t/p/w780" + path
}
