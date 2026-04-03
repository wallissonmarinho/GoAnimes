package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPStatusError is returned when the server responds with a non-2xx status.
type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("http %d", e.StatusCode)
}

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
	b, err := g.GetBytesGETRetry(context.Background(), url, 1, 0)
	return b, err
}

func (g *Getter) doGET(ctx context.Context, url string) (body []byte, status int, hdr http.Header, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	if g.UserAgent != "" {
		req.Header.Set("User-Agent", g.UserAgent)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	hdr = resp.Header.Clone()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, status, hdr, &HTTPStatusError{StatusCode: status}
	}
	r := io.LimitReader(resp.Body, g.MaxBodyBytes+1)
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, status, hdr, err
	}
	if int64(len(b)) > g.MaxBodyBytes {
		return nil, status, hdr, fmt.Errorf("body exceeds max bytes")
	}
	return b, status, hdr, nil
}

// parseRetryAfter returns wait duration from Retry-After (seconds or HTTP-date), capped.
func parseRetryAfter(h http.Header) (time.Duration, bool) {
	ra := strings.TrimSpace(h.Get("Retry-After"))
	if ra == "" {
		return 0, false
	}
	if sec, err := strconv.Atoi(ra); err == nil {
		if sec < 0 {
			sec = 0
		}
		if sec > 120 {
			sec = 120
		}
		return time.Duration(sec) * time.Second, true
	}
	if t, err := http.ParseTime(ra); err == nil {
		d := time.Until(t)
		if d < time.Second {
			return time.Second, true
		}
		if d > 120*time.Second {
			return 120 * time.Second, true
		}
		return d, true
	}
	return 0, false
}

func retryWaitAfterThrottle(failedAttempt int, baseBackoff time.Duration, h http.Header) time.Duration {
	const maxW = 90 * time.Second
	if d, ok := parseRetryAfter(h); ok && d > 0 {
		if d > maxW {
			return maxW
		}
		return d
	}
	if baseBackoff <= 0 {
		baseBackoff = 2 * time.Second
	}
	mult := 1 << uint(failedAttempt-1)
	if mult < 1 {
		mult = 1
	}
	w := baseBackoff * time.Duration(mult)
	if w > maxW {
		return maxW
	}
	return w
}

// isRetriableTransportErr is true for client timeouts and similar transient transport failures (not 4xx/5xx bodies).
func isRetriableTransportErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	// http.Client Timeout / TLS handshake timeouts often surface as url.Error wrapping deadline.
	var s string
	for e := err; e != nil; e = errors.Unwrap(e) {
		s = strings.ToLower(e.Error())
		if strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded") {
			return true
		}
	}
	return false
}

// GetBytesGETRetry performs a GET and retries on 429 / 503 with exponential backoff or Retry-After,
// and on client timeouts / connection timeouts (same attempt budget).
// maxAttempts must be >= 1. baseBackoff is the first backoff when Retry-After is absent (default 2s if <= 0).
func (g *Getter) GetBytesGETRetry(ctx context.Context, url string, maxAttempts int, baseBackoff time.Duration) ([]byte, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, status, hdr, err := g.doGET(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if attempt >= maxAttempts {
			break
		}
		var st *HTTPStatusError
		if errors.As(err, &st) {
			if status != http.StatusTooManyRequests && status != http.StatusServiceUnavailable {
				break
			}
			wait := retryWaitAfterThrottle(attempt, baseBackoff, hdr)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if !isRetriableTransportErr(err) {
			break
		}
		wait := retryWaitAfterThrottle(attempt, baseBackoff, nil)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

// PostBytes sends a POST with a body and returns the response body (capped).
func (g *Getter) PostBytes(url string, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
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
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
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
