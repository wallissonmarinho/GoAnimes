package thetvdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const apiRoot = "https://api4.thetvdb.com/v4"

// Client calls TheTVDB API v4 (Bearer from /login). See https://www.thetvdb.com/api-information
type Client struct {
	getter *httpclient.Getter
	apiKey string
	pin    string

	mu          sync.Mutex
	bearerToken string
	tokenAt     time.Time
}

// NewClient returns nil if getter or apiKey is empty.
func NewClient(g *httpclient.Getter, apiKey, subscriberPIN string) *Client {
	if g == nil || strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &Client{getter: g, apiKey: strings.TrimSpace(apiKey), pin: strings.TrimSpace(subscriberPIN)}
}

type loginReq struct {
	APIKey string `json:"apikey"`
	PIN    string `json:"pin,omitempty"` // subscriber PIN when required by the key tier
}

type loginResp struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
	Status string `json:"status"`
}

func (c *Client) ensureToken(ctx context.Context) error {
	if c == nil {
		return errors.New("thetvdb: nil client")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bearerToken != "" && time.Since(c.tokenAt) < 20*24*time.Hour {
		return nil
	}
	body := loginReq{APIKey: c.apiKey}
	if c.pin != "" {
		body.PIN = c.pin
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiRoot+"/login", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.getter.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("thetvdb login: http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out loginResp
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	tok := strings.TrimSpace(out.Data.Token)
	if tok == "" {
		return errors.New("thetvdb login: empty token")
	}
	c.bearerToken = tok
	c.tokenAt = time.Now().UTC()
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if err := c.ensureToken(ctx); err != nil {
			return nil, err
		}
		b, err := c.getJSONOnce(ctx, path)
		if err != nil {
			var st *httpclient.HTTPStatusError
			if errors.As(err, &st) && st.StatusCode == http.StatusUnauthorized && attempt == 0 {
				c.mu.Lock()
				c.bearerToken = ""
				c.mu.Unlock()
				continue
			}
			return nil, err
		}
		return b, nil
	}
	return nil, errors.New("thetvdb: get exhausted retries")
}

func (c *Client) getJSONOnce(ctx context.Context, path string) ([]byte, error) {
	u := apiRoot + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	tok := c.bearerToken
	c.mu.Unlock()
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := c.getter.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	max := c.getter.MaxBodyBytes
	if max <= 0 {
		max = 8 << 20
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("thetvdb: response too large")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &httpclient.HTTPStatusError{StatusCode: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpclient.HTTPStatusError{StatusCode: resp.StatusCode}
	}
	return b, nil
}

type searchRemoteResp struct {
	Data []struct {
		Series *struct {
			ID int `json:"id"`
		} `json:"series"`
	} `json:"data"`
	Status string `json:"status"`
}

// SeriesIDByIMDbRemote returns TheTVDB series id for an IMDb tt… id, or 0 if not found.
func (c *Client) SeriesIDByIMDbRemote(ctx context.Context, imdbTT string) (int, error) {
	if c == nil {
		return 0, errors.New("thetvdb: nil client")
	}
	imdbTT = domain.NormalizeIMDbID(imdbTT)
	if imdbTT == "" {
		return 0, nil
	}
	path := "/search/remoteid/" + url.PathEscape(imdbTT)
	b, err := c.getJSON(ctx, path)
	if err != nil {
		return 0, err
	}
	var out searchRemoteResp
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, err
	}
	for _, row := range out.Data {
		if row.Series != nil && row.Series.ID > 0 {
			return row.Series.ID, nil
		}
	}
	return 0, nil
}

type episodePage struct {
	Data struct {
		Episodes []struct {
			ID             int    `json:"id"`
			Name           string `json:"name"`
			Number         int    `json:"number"`
			SeasonNumber   int    `json:"seasonNumber"`
			SeasonName     string `json:"seasonName"`
			Image          string `json:"image"`
			AbsoluteNumber *int   `json:"absoluteNumber"`
		} `json:"episodes"`
	} `json:"data"`
	Status string `json:"status"`
}

// EpisodeMapsOfficial returns episode titles and thumbnails keyed by flat episode number (season 1 only),
// matching GoAnimes AniList/Jikan episode maps.
func (c *Client) EpisodeMapsOfficial(ctx context.Context, seriesID int) (titles, thumbs map[int]string, err error) {
	if seriesID <= 0 {
		return nil, nil, nil
	}
	titles = make(map[int]string)
	thumbs = make(map[int]string)
	for page := 0; page < 200; page++ {
		path := fmt.Sprintf("/series/%d/episodes/default?page=%d", seriesID, page)
		b, gerr := c.getJSON(ctx, path)
		if gerr != nil {
			return titles, thumbs, gerr
		}
		var ep episodePage
		if err := json.Unmarshal(b, &ep); err != nil {
			return titles, thumbs, err
		}
		eps := ep.Data.Episodes
		if len(eps) == 0 {
			break
		}
		for _, e := range eps {
			if e.SeasonNumber != 1 {
				continue
			}
			if e.Number <= 0 {
				continue
			}
			name := strings.TrimSpace(e.Name)
			if name != "" {
				titles[e.Number] = name
			}
			img := normalizeArtworkURL(e.Image)
			if img != "" {
				thumbs[e.Number] = img
			}
		}
		if len(eps) < 50 {
			break
		}
	}
	return titles, thumbs, nil
}

type seriesExtendedResp struct {
	Data struct {
		Artworks []struct {
			Image  string  `json:"image"`
			Width  int     `json:"width"`
			Height int     `json:"height"`
			Type   int     `json:"type"`
			Score  float64 `json:"score"`
		} `json:"artworks"`
	} `json:"data"`
	Status string `json:"status"`
}

func normalizeArtworkURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://") {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	return "https://artworks.thetvdb.com" + strings.TrimPrefix(s, "/")
}

// SeriesFanartCandidates returns wide artwork candidates (fanart / keyart types when width known).
func (c *Client) SeriesFanartCandidates(ctx context.Context, seriesID int) ([]domain.BackgroundCandidate, error) {
	if c == nil || seriesID <= 0 {
		return nil, nil
	}
	b, err := c.getJSON(ctx, fmt.Sprintf("/series/%d/extended", seriesID))
	if err != nil {
		return nil, err
	}
	var ext seriesExtendedResp
	if err := json.Unmarshal(b, &ext); err != nil {
		return nil, err
	}
	// TVDB artwork type ids: 14 poster, 15 background/banner, 16 fanart — use fanart (16) and background-like (15).
	const (
		typeBackground = 15
		typeFanart     = 16
	)
	type scored struct {
		d domain.BackgroundCandidate
		v float64
	}
	var tmp []scored
	for _, a := range ext.Data.Artworks {
		if a.Type != typeFanart && a.Type != typeBackground {
			continue
		}
		u := normalizeArtworkURL(a.Image)
		if u == "" {
			continue
		}
		w, h := a.Width, a.Height
		if w <= 0 || h <= 0 {
			w, h = 1920, 1080
		}
		tmp = append(tmp, scored{d: domain.BackgroundCandidate{URL: u, W: w, H: h}, v: a.Score})
	}
	if len(tmp) == 0 {
		return nil, nil
	}
	sort.SliceStable(tmp, func(i, j int) bool { return tmp[i].v > tmp[j].v })
	const capN = 12
	if len(tmp) > capN {
		tmp = tmp[:capN]
	}
	out := make([]domain.BackgroundCandidate, 0, len(tmp))
	for _, t := range tmp {
		out = append(out, t.d)
	}
	return out, nil
}
