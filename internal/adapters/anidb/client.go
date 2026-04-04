package anidb

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
)

const defaultBase = "http://api.anidb.net:9001/httpapi"

// AniDB HTTP API requires a registered client (https://anidb.net/perl-bin/animedb.pl?show=client).
// Wiki: max ~1 request / 2s; cache aggressively — repeat same-day fetches risk a ban.
const defaultMinRequestInterval = 2100 * time.Millisecond

var anidbURLPathRe = regexp.MustCompile(`(?i)anidb\.net/anime/(\d+)`)

// Client calls AniDB HTTP XML API (request=anime) for episode titles.
type Client struct {
	getter      *httpclient.Getter
	base        string
	client      string
	clientVer   int
	minInterval time.Duration
	mu          sync.Mutex
	lastEnd     time.Time
}

// Option configures Client.
type Option func(*Client)

// WithBaseURL sets the API root (tests).
func WithBaseURL(base string) Option {
	return func(c *Client) {
		if strings.TrimSpace(base) != "" {
			c.base = strings.TrimSuffix(strings.TrimSpace(base), "/")
		}
	}
}

// WithMinRequestInterval sets minimum delay between requests (0 = no pacing; tests only).
func WithMinRequestInterval(d time.Duration) Option {
	return func(c *Client) {
		c.minInterval = d
	}
}

// NewClient returns nil if getter is nil, client name empty, or clientVer < 1.
func NewClient(g *httpclient.Getter, clientName string, clientVer int, opts ...Option) *Client {
	if g == nil || g.Client == nil || strings.TrimSpace(clientName) == "" || clientVer < 1 {
		return nil
	}
	c := &Client{
		getter:      g,
		base:        defaultBase,
		client:      strings.ToLower(strings.TrimSpace(clientName)),
		clientVer:   clientVer,
		minInterval: defaultMinRequestInterval,
	}
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

// FetchEpisodeTitlesByAID returns regular episode numbers (epno type 1) → title (prefers en, then x-jat, ja).
func (c *Client) FetchEpisodeTitlesByAID(ctx context.Context, aid int) (map[int]string, error) {
	if c == nil || c.getter == nil {
		return nil, errors.New("anidb: nil client")
	}
	if aid < 1 {
		return nil, errors.New("anidb: invalid aid")
	}
	if err := c.pace(ctx); err != nil {
		return nil, err
	}
	q := url.Values{
		"request":   {"anime"},
		"client":    {c.client},
		"clientver": {strconv.Itoa(c.clientVer)},
		"protover":  {"1"},
		"aid":       {strconv.Itoa(aid)},
	}
	reqURL := c.base + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml, text/xml, */*")
	if c.getter.UserAgent != "" {
		req.Header.Set("User-Agent", c.getter.UserAgent)
	}
	resp, err := c.getter.Client.Do(req)
	c.noteReqDone()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	r := io.LimitReader(resp.Body, c.getter.MaxBodyBytes+1)
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.getter.MaxBodyBytes {
		return nil, fmt.Errorf("anidb: body exceeds max bytes")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpclient.HTTPStatusError{StatusCode: resp.StatusCode}
	}
	return parseAnimeEpisodeTitles(body)
}

type apiErrorXML struct {
	XMLName xml.Name `xml:"error"`
	Text    string   `xml:",chardata"`
}

type animeRootXML struct {
	XMLName  xml.Name    `xml:"anime"`
	Episodes episodesXML `xml:"episodes"`
}

type episodesXML struct {
	Episode []episodeXML `xml:"episode"`
}

type episodeXML struct {
	ID    string    `xml:"id,attr"`
	Epno  epnoXML   `xml:"epno"`
	Title []titleEl `xml:"title"`
}

type epnoXML struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type titleEl struct {
	Lang string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Text string `xml:",chardata"`
}

func parseAnimeEpisodeTitles(body []byte) (map[int]string, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, errors.New("anidb: empty xml")
	}
	if bytes.Contains(body, []byte("<error")) {
		var e apiErrorXML
		if err := xml.Unmarshal(body, &e); err == nil && strings.TrimSpace(e.Text) != "" {
			return nil, fmt.Errorf("anidb: %s", strings.TrimSpace(e.Text))
		}
	}
	var root animeRootXML
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if root.XMLName.Local != "anime" && len(root.Episodes.Episode) == 0 {
		return nil, fmt.Errorf("anidb: unexpected xml (not anime)")
	}
	out := make(map[int]string)
	for _, ep := range root.Episodes.Episode {
		if strings.TrimSpace(ep.Epno.Type) != "1" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(ep.Epno.Text))
		if err != nil || n < 1 {
			continue
		}
		if t := pickEpisodeTitle(ep.Title); t != "" {
			if _, ok := out[n]; !ok {
				out[n] = t
			}
		}
	}
	return out, nil
}

func pickEpisodeTitle(titles []titleEl) string {
	if len(titles) == 0 {
		return ""
	}
	pref := []string{"en", "x-jat", "ja", "de", "fr", "es", "it", "pt"}
	for _, want := range pref {
		for _, t := range titles {
			if strings.EqualFold(strings.TrimSpace(t.Lang), want) {
				if s := strings.TrimSpace(t.Text); s != "" {
					return s
				}
			}
		}
	}
	for _, t := range titles {
		if s := strings.TrimSpace(t.Text); s != "" {
			return s
		}
	}
	return ""
}

// ParseAnidbAidFromURL extracts anime id from an AniDB URL (e.g. https://anidb.net/anime/19614).
func ParseAnidbAidFromURL(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if m := anidbURLPathRe.FindStringSubmatch(raw); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
