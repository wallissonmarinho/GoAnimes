package rss

import (
	"context"
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

// Match common Portuguese subtitle markers: pt, pt-br, ptbr, pt_pt, ptpt, portuguese, brazilian portuguese, [br]
var portugueseSubtitleRe = regexp.MustCompile(`(?i)(\[br\]|\bpt(?:[-_ ]?br|[-_ ]?pt)?\b|\bportuguese\b|\bbrazilian portuguese\b)`)

func NewReader() *Reader {
	return &Reader{Parser: gofeed.NewParser()}
}

func (r *Reader) Fetch(ctx context.Context, feed domain.Feed) ([]ports.ReleaseItem, error) {
	if feed.Type != domain.FeedTypeRSS && feed.Type != domain.FeedTypeTorznab {
		return []ports.ReleaseItem{}, nil
	}
	fp := r.Parser
	if fp == nil {
		fp = gofeed.NewParser()
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
	for _, enclosure := range item.Enclosures {
		if url := strings.TrimSpace(enclosure.URL); url != "" {
			return url
		}
	}
	return strings.TrimSpace(item.Link)
}

func hasPortugueseSubtitle(item *gofeed.Item) bool {
	if item == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{item.Title, item.Description, item.Content}, " ")))
	return portugueseSubtitleRe.MatchString(text)
}
