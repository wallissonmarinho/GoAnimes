package translate

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
	"gopkg.gilang.dev/translator/v2/googletranslate"
)

// GilangGoogle wraps gopkg.gilang.dev/translator/v2/googletranslate (MIT).
// See https://github.com/gilang-as/translator
type GilangGoogle struct {
	inner   *googletranslate.GoogleTranslate
	timeout time.Duration
}

var _ ports.SynopsisTranslator = (*GilangGoogle)(nil)

// escapeForGilangGoogleRPCJSON escapes text for the inner payload gilang builds with
// fmt.Sprintf(`[["%s","%s","%s",true],[null]]`, text, from, to). Raw " or \ or newlines break the RPC JSON
// and surface as "request on google translate api isn't working".
func escapeForGilangGoogleRPCJSON(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NewGilangGoogle builds a Google Translate client via the gilang library (default translate.google.com host).
func NewGilangGoogle(timeout time.Duration) *GilangGoogle {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &GilangGoogle{
		inner: googletranslate.New(
			googletranslate.WithHTTPClient(&http.Client{Timeout: timeout}),
		),
		timeout: timeout,
	}
}

func (g *GilangGoogle) Name() string { return "gilang-googletranslate" }

func (g *GilangGoogle) Translate(text, source, target string) (string, error) {
	if g == nil || g.inner == nil {
		return "", errors.New("gilang google: nil client")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("gilang google: empty text")
	}
	text = escapeForGilangGoogleRPCJSON(text)
	source = strings.TrimSpace(source)
	if source == "" {
		source = "auto"
	}
	target = strings.TrimSpace(target)
	if target == "" {
		target = "pt"
	}
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	res, err := g.inner.Translate(ctx, text, source, target)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", errors.New("gilang google: nil result")
	}
	out := strings.TrimSpace(res.Text)
	if out == "" {
		return "", errors.New("gilang google: empty translation")
	}
	return out, nil
}

// timeoutFromGetter returns a positive duration from the shared HTTP getter, or defaultTimeout.
func timeoutFromGetter(getter *httpclient.Getter, defaultTimeout time.Duration) time.Duration {
	if getter != nil && getter.Client != nil && getter.Client.Timeout > 0 {
		return getter.Client.Timeout
	}
	if defaultTimeout <= 0 {
		return 45 * time.Second
	}
	return defaultTimeout
}

// NewSynopsisTranslator returns the default synopsis translator (gilang → Google). Nil only if getter is nil.
func NewSynopsisTranslator(getter *httpclient.Getter) ports.SynopsisTranslator {
	if getter == nil {
		return nil
	}
	return NewGilangGoogle(timeoutFromGetter(getter, 45*time.Second))
}
