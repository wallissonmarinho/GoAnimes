package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Getter downloads HTTP bodies with a size limit.
type Getter struct {
	Client     *http.Client
	UserAgent  string
	MaxBodyBytes int64
}

func NewGetter(timeout time.Duration, userAgent string, maxBodyBytes int64) *Getter {
	if maxBodyBytes <= 0 {
		maxBodyBytes = 50 << 20
	}
	return &Getter{
		Client: &http.Client{Timeout: timeout},
		UserAgent: strings.TrimSpace(userAgent),
		MaxBodyBytes: maxBodyBytes,
	}
}

// GetBytes fetches url and returns body (capped).
func (g *Getter) GetBytes(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if g.UserAgent != "" {
		req.Header.Set("User-Agent", g.UserAgent)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	r := io.LimitReader(resp.Body, g.MaxBodyBytes+1)
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > g.MaxBodyBytes {
		return nil, fmt.Errorf("body exceeds max bytes")
	}
	return b, nil
}
