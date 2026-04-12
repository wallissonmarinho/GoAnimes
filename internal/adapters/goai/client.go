package goai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// Client implements ports.GoAIAuditHTTPClient against a GoAI base URL.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

// NewClient builds a client. baseURL should be like https://goai.example.com (no trailing slash).
func NewClient(baseURL, apiKey string, timeout time.Duration, userAgent string) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		baseURL:    b,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  strings.TrimSpace(userAgent),
	}
}

var _ ports.GoAIAuditHTTPClient = (*Client)(nil)

func (c *Client) postJSON(ctx context.Context, path string, body any, out any) error {
	if c.baseURL == "" || c.apiKey == "" {
		return fmt.Errorf("goai: missing base URL or API key")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("goai: %s %s: %s", path, res.Status, truncateForErr(b, 512))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("goai: decode %s: %w body=%s", path, err, truncateForErr(b, 256))
	}
	return nil
}

func truncateForErr(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// AuditSeries calls POST /v1/audit/series.
func (c *Client) AuditSeries(ctx context.Context, req domain.GoaiSeriesAuditRequest) (*domain.GoaiSeriesAuditResponse, error) {
	var resp domain.GoaiSeriesAuditResponse
	if err := c.postJSON(ctx, "/v1/audit/series", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AuditRelease calls POST /v1/audit/release.
func (c *Client) AuditRelease(ctx context.Context, req domain.GoaiReleaseAuditRequest) (*domain.GoaiReleaseAuditResponse, error) {
	var resp domain.GoaiReleaseAuditResponse
	if err := c.postJSON(ctx, "/v1/audit/release", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
