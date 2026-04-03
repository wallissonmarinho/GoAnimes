package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const apiRoot = "https://api.themoviedb.org/3"
const imageW1280 = "https://image.tmdb.org/t/p/w1280"

// Client queries TMDB v3 (free API key) for backdrop images with known pixel dimensions.
type Client struct {
	getter *httpclient.Getter
	key    string
}

// NewClient returns nil if getter or apiKey is empty.
func NewClient(g *httpclient.Getter, apiKey string) *Client {
	if g == nil || strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &Client{getter: g, key: strings.TrimSpace(apiKey)}
}

type findResp struct {
	MovieResults []struct {
		ID int `json:"id"`
	} `json:"movie_results"`
	TVResults []struct {
		ID int `json:"id"`
	} `json:"tv_results"`
}

type imagesResp struct {
	Backdrops []struct {
		FilePath    string  `json:"file_path"`
		Width       int     `json:"width"`
		Height      int     `json:"height"`
		VoteAverage float64 `json:"vote_average"`
	} `json:"backdrops"`
}

type searchTVResp struct {
	Results []struct {
		ID int `json:"id"`
	} `json:"results"`
}

func (c *Client) getJSON(ctx context.Context, u string) ([]byte, error) {
	if c == nil || c.getter == nil {
		return nil, errors.New("tmdb: nil client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.getter.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, &httpclient.HTTPStatusError{StatusCode: resp.StatusCode}
	}
	max := c.getter.MaxBodyBytes
	if max <= 0 {
		max = 2 << 20
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("tmdb: response too large")
	}
	return b, nil
}

func backdropsToCandidates(img imagesResp) []domain.BackgroundCandidate {
	if len(img.Backdrops) == 0 {
		return nil
	}
	type scored struct {
		d domain.BackgroundCandidate
		v float64
	}
	var tmp []scored
	for _, b := range img.Backdrops {
		fp := strings.TrimSpace(b.FilePath)
		if fp == "" || b.Width <= 0 || b.Height <= 0 {
			continue
		}
		u := imageW1280 + fp
		tmp = append(tmp, scored{
			d: domain.BackgroundCandidate{URL: u, W: b.Width, H: b.Height},
			v: b.VoteAverage,
		})
	}
	if len(tmp) == 0 {
		return nil
	}
	sort.SliceStable(tmp, func(i, j int) bool { return tmp[i].v > tmp[j].v })
	const capN = 10
	if len(tmp) > capN {
		tmp = tmp[:capN]
	}
	out := make([]domain.BackgroundCandidate, 0, len(tmp))
	for _, t := range tmp {
		out = append(out, t.d)
	}
	return out
}

// BackdropCandidatesForIMDB uses TMDB /find by IMDb id, then loads TV or movie images.
func (c *Client) BackdropCandidatesForIMDB(ctx context.Context, imdbTT string) ([]domain.BackgroundCandidate, error) {
	if c == nil {
		return nil, errors.New("tmdb: nil client")
	}
	imdbTT = domain.NormalizeIMDbID(imdbTT)
	if imdbTT == "" {
		return nil, errors.New("tmdb: missing imdb id")
	}
	u := apiRoot + "/find/" + url.PathEscape(imdbTT) + "?" + url.Values{
		"api_key":           {c.key},
		"external_source":   {"imdb_id"},
	}.Encode()
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return nil, err
	}
	var fr findResp
	if err := json.Unmarshal(body, &fr); err != nil {
		return nil, err
	}
	if len(fr.TVResults) > 0 && fr.TVResults[0].ID > 0 {
		return c.backdropCandidatesTV(ctx, fr.TVResults[0].ID)
	}
	if len(fr.MovieResults) > 0 && fr.MovieResults[0].ID > 0 {
		return c.backdropCandidatesMovie(ctx, fr.MovieResults[0].ID)
	}
	return nil, nil
}

// BackdropCandidatesForTVSearch searches TV by title (+ optional first_air_date_year) and returns backdrop candidates.
func (c *Client) BackdropCandidatesForTVSearch(ctx context.Context, query string, year int) ([]domain.BackgroundCandidate, error) {
	if c == nil {
		return nil, errors.New("tmdb: nil client")
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("tmdb: empty query")
	}
	v := url.Values{
		"api_key": {c.key},
		"query":   {q},
	}
	if year > 0 {
		v.Set("first_air_date_year", strconv.Itoa(year))
	}
	u := apiRoot + "/search/tv?" + v.Encode()
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return nil, err
	}
	var sr searchTVResp
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}
	if len(sr.Results) == 0 || sr.Results[0].ID <= 0 {
		return nil, nil
	}
	return c.backdropCandidatesTV(ctx, sr.Results[0].ID)
}

func (c *Client) backdropCandidatesTV(ctx context.Context, tvID int) ([]domain.BackgroundCandidate, error) {
	u := apiRoot + "/tv/" + strconv.Itoa(tvID) + "/images?" + url.Values{"api_key": {c.key}}.Encode()
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return nil, err
	}
	var ir imagesResp
	if err := json.Unmarshal(body, &ir); err != nil {
		return nil, err
	}
	return backdropsToCandidates(ir), nil
}

func (c *Client) backdropCandidatesMovie(ctx context.Context, movieID int) ([]domain.BackgroundCandidate, error) {
	u := apiRoot + "/movie/" + strconv.Itoa(movieID) + "/images?" + url.Values{"api_key": {c.key}}.Encode()
	body, err := c.getJSON(ctx, u)
	if err != nil {
		return nil, err
	}
	var ir imagesResp
	if err := json.Unmarshal(body, &ir); err != nil {
		return nil, err
	}
	return backdropsToCandidates(ir), nil
}