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

var portugueseSubtitleRe = regexp.MustCompile(`(?i)(\[br\]|\bpt[-_ ]?br\b|\bbrazilian portuguese\b|\bportuguese\b)`)

func NewReader() *Reader {
	return &Reader{Parser: gofeed.NewParser()}
}

func (r *Reader) Fetch(ctx context.Context, feed domain.Feed) ([]ports.ReleaseItem, error) {
	if feed.Type != domain.FeedTypeRSS {
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
			Link:      strings.TrimSpace(it.Link),
			Provider:  feed.Name,
			Quality:   "",
			Published: published,
		})
	}
	return items, nil
}

func hasPortugueseSubtitle(item *gofeed.Item) bool {
	if item == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{item.Title, item.Description, item.Content}, " ")))
	return portugueseSubtitleRe.MatchString(text)
}
