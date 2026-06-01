package rss

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
)

type Reader struct {
	Parser *gofeed.Parser
}

const defaultHTTPTimeout = 45 * time.Second

// Match common Portuguese subtitle markers: pt, pt-br, ptbr, pt_pt, ptpt, portuguese, brazilian portuguese, [br]
var portugueseSubtitleRe = regexp.MustCompile(`(?i)(\[br\]|\bpt(?:[-_ ]?br|[-_ ]?pt)?\b|\bportuguese\b|\bbrazilian portuguese\b)`)
var magnetURLRe = regexp.MustCompile(`magnet:\?xt=urn:btih:[^\s<>"']+`)

func NewReader() *Reader {
	parser := gofeed.NewParser()
	parser.Client = &http.Client{Timeout: defaultHTTPTimeout}
	return &Reader{Parser: parser}
}

func (r *Reader) Fetch(ctx context.Context, feed domain.Feed) ([]ports.ReleaseItem, error) {
	if feed.Type != domain.FeedTypeRSS && feed.Type != domain.FeedTypeTorznab {
		return []ports.ReleaseItem{}, nil
	}
	fp := r.Parser
	if fp == nil {
		fp = gofeed.NewParser()
	}
	if fp.Client == nil {
		fp.Client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	parsed, err := fp.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ports.ReleaseItem, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		if !hasPortugueseSubtitle(it) {
			continue
		}
		published := time.Now().UTC()
		if it.PublishedParsed != nil {
			published = it.PublishedParsed.UTC()
		}
		items = append(items, ports.ReleaseItem{
			Title:     strings.TrimSpace(it.Title),
			Magnet:    "",
			Link:      pickDownloadURL(it),
			Provider:  feed.Name,
			Quality:   "",
			Published: published,
		})
	}
	return items, nil
}

func pickDownloadURL(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	if magnet := pickMagnetURL(item); magnet != "" {
		return magnet
	}
	for _, enclosure := range item.Enclosures {
		if url := strings.TrimSpace(enclosure.URL); url != "" {
			return url
		}
	}
	return strings.TrimSpace(item.Link)
}

func pickMagnetURL(item *gofeed.Item) string {
	for _, enclosure := range item.Enclosures {
		if url := cleanMagnetURL(enclosure.URL); url != "" {
			return url
		}
	}
	for _, extensionGroup := range item.Extensions {
		for _, extensions := range extensionGroup {
			for _, extension := range extensions {
				for key, value := range extension.Attrs {
					if strings.EqualFold(strings.TrimSpace(key), "magneturl") {
						if url := cleanMagnetURL(value); url != "" {
							return url
						}
					}
				}
				if url := cleanMagnetURL(extension.Value); url != "" {
					return url
				}
			}
		}
	}
	if url := cleanMagnetURL(strings.Join([]string{item.Link, item.Description, item.Content}, " ")); url != "" {
		return url
	}
	return ""
}

func cleanMagnetURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "magnet:") {
		return strings.TrimRight(value, ".,;")
	}
	if match := magnetURLRe.FindString(value); match != "" {
		return strings.TrimRight(match, ".,;")
	}
	return ""
}

func hasPortugueseSubtitle(item *gofeed.Item) bool {
	if item == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{item.Title, item.Description, item.Content}, " ")))
	return portugueseSubtitleRe.MatchString(text)
}
