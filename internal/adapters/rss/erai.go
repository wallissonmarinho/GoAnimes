package rss

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/mmcdole/gofeed"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

const (
	// StremioIDPrefix is the manifest idPrefix for custom meta/stream ids.
	StremioIDPrefix = "goanimes"
	// StremioMetaType is the per-item type Stremio uses for meta/stream (playback).
	// Using "movie" here avoids clients that fail playback when meta type is only "anime"; catalog entries stay "anime".
	StremioMetaType = "movie"
)

var (
	eraiSubtitlesRe = regexp.MustCompile(`(?i)<erai:subtitles[^>]*>([^<]*)</erai:subtitles>`)
	magnetHashRe    = regexp.MustCompile(`(?i)btih:([a-f0-9]{40})`)
)

// ParseFeed parses RSS/Atom XML and returns catalog items that include [br] in Erai subtitles.
func ParseFeed(body []byte) ([]domain.CatalogItem, error) {
	items, _, err := ParseFeedWithEraiSlugs(body)
	return items, err
}

// ParseFeedWithEraiSlugs is like ParseFeed but also returns Erai /anime-list/{slug}/ segments found in item links/HTML (for per-anime RSS expansion).
func ParseFeedWithEraiSlugs(body []byte) ([]domain.CatalogItem, []string, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(string(body))
	if err != nil {
		items, slugs, err2 := parseFallbackXMLWithSlugs(body)
		if err2 == nil && len(items) > 0 {
			return items, slugs, nil
		}
		if err2 != nil && !errors.Is(err2, io.EOF) {
			return nil, nil, err2
		}
		return nil, nil, err
	}
	raw := string(body)
	var slugAcc []string
	for _, item := range feed.Items {
		slugAcc = append(slugAcc, discoverSlugsFromGofeedItem(raw, item)...)
	}
	var out []domain.CatalogItem
	for _, item := range feed.Items {
		subTag := eraiSubtitlesFromExtensions(item)
		if subTag == "" {
			subTag = eraiSubtitlesFromRaw(raw, item)
		}
		if !strings.Contains(subTag, "[br]") {
			continue
		}
		magnet, torrent, hash := resolveStream(item)
		if magnet == "" && torrent == "" && hash == "" {
			continue
		}
		id := stableItemID(item)
		name := strings.TrimSpace(item.Title)
		if name == "" {
			name = "Untitled"
		}
		released := ""
		if item.PublishedParsed != nil {
			released = item.PublishedParsed.Format("2006-01-02")
		}
		out = append(out, domain.CatalogItem{
			ID:           StremioIDPrefix + ":" + id,
			Type:         StremioMetaType,
			Name:         name,
			MagnetURL:    magnet,
			TorrentURL:   torrent,
			InfoHash:     hash,
			Released:     strings.TrimSpace(released),
			SubtitlesTag: subTag,
		})
	}
	return out, uniqueEraiSlugs(slugAcc), nil
}

func eraiSubtitlesFromExtensions(item *gofeed.Item) string {
	if item.Extensions == nil {
		return ""
	}
	if m, ok := item.Extensions["erai"]; ok {
		if v, ok := m["subtitles"]; ok && len(v) > 0 {
			var parts []string
			for _, e := range v {
				parts = append(parts, e.Value)
			}
			return strings.Join(parts, "")
		}
	}
	for _, sub := range item.Extensions {
		if v, ok := sub["subtitles"]; ok && len(v) > 0 {
			var parts []string
			for _, e := range v {
				parts = append(parts, e.Value)
			}
			return strings.Join(parts, "")
		}
	}
	return ""
}

// itemXMLBlock returns the raw <item>...</item> fragment for a gofeed item (for custom tags + slug discovery).
func itemXMLBlock(raw string, item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	guid := strings.TrimSpace(item.GUID)
	link := strings.TrimSpace(item.Link)
	title := strings.TrimSpace(item.Title)
	parts := strings.Split(raw, "<item")
	for _, p := range parts[1:] {
		block := "<item" + p
		if end := strings.Index(block, "</item>"); end >= 0 {
			block = block[:end+7]
		}
		if link != "" && strings.Contains(block, link) {
			return block
		}
		if guid != "" && strings.Contains(block, guid) {
			return block
		}
		if title != "" && strings.Contains(block, title) {
			return block
		}
	}
	return ""
}

func eraiSubtitlesFromRaw(raw string, item *gofeed.Item) string {
	return extractEraiSubtitles(itemXMLBlock(raw, item))
}

func extractEraiSubtitles(s string) string {
	m := eraiSubtitlesRe.FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func resolveStream(item *gofeed.Item) (magnet, torrent, hash string) {
	link := strings.TrimSpace(item.Link)
	if strings.HasPrefix(strings.ToLower(link), "magnet:") {
		magnet = link
		if m := magnetHashRe.FindStringSubmatch(link); len(m) > 1 {
			hash = strings.ToLower(m[1])
		}
		return
	}
	for _, enc := range item.Enclosures {
		u := strings.TrimSpace(enc.URL)
		if u == "" {
			continue
		}
		typ := strings.ToLower(enc.Type)
		if typ == "application/x-bittorrent" || strings.HasSuffix(strings.ToLower(u), ".torrent") {
			torrent = u
			return
		}
	}
	if link != "" && (strings.HasSuffix(strings.ToLower(link), ".torrent") || strings.Contains(strings.ToLower(link), "torrent")) {
		torrent = link
	}
	return
}

func stableItemID(item *gofeed.Item) string {
	g := strings.TrimSpace(item.GUID)
	if g == "" {
		g = strings.TrimSpace(item.Link)
	}
	if g == "" {
		g = item.Title
	}
	sum := sha256.Sum256([]byte(g))
	return hex.EncodeToString(sum[:])
}

func parseFallbackXMLWithSlugs(body []byte) ([]domain.CatalogItem, []string, error) {
	var doc struct {
		Channel struct {
			Items []struct {
				Title     string `xml:"title"`
				Link      string `xml:"link"`
				Guid      string `xml:"guid"`
				PubDate   string `xml:"pubDate"`
				Enclosure []struct {
					URL  string `xml:"url,attr"`
					Type string `xml:"type,attr"`
				} `xml:"enclosure"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Channel.Items) == 0 {
		return nil, nil, io.EOF
	}
	raw := string(body)
	var slugAcc []string
	var out []domain.CatalogItem
	for _, it := range doc.Channel.Items {
		var block string
		if strings.TrimSpace(it.Link) != "" {
			parts := strings.Split(raw, "<item")
			for _, p := range parts[1:] {
				b := "<item" + p
				if strings.Contains(b, strings.TrimSpace(it.Link)) {
					block = b
					break
				}
			}
		}
		slugAcc = append(slugAcc, ExtractEraiAnimeListSlugs(block, it.Link, it.Guid, it.Title)...)
		slugAcc = append(slugAcc, ExtractEraiAnimeListSlugsFromEpisodeLinks(block, it.Link, it.Guid, it.Title)...)
		slugAcc = append(slugAcc, ExtractEraiAnimeListSlugsFromEncodesLinks(block, it.Link, it.Guid, it.Title)...)
		sub := extractEraiSubtitles(block)
		if sub == "" {
			continue
		}
		if !strings.Contains(sub, "[br]") {
			continue
		}
		magnet, torrent, hash := "", "", ""
		link := strings.TrimSpace(it.Link)
		if strings.HasPrefix(strings.ToLower(link), "magnet:") {
			magnet = link
			if m := magnetHashRe.FindStringSubmatch(link); len(m) > 1 {
				hash = strings.ToLower(m[1])
			}
		}
		for _, enc := range it.Enclosure {
			u := strings.TrimSpace(enc.URL)
			if u == "" {
				continue
			}
			if strings.Contains(strings.ToLower(enc.Type), "bittorrent") || strings.HasSuffix(strings.ToLower(u), ".torrent") {
				torrent = u
				break
			}
		}
		if magnet == "" && torrent == "" && hash == "" {
			continue
		}
		g := strings.TrimSpace(it.Guid)
		if g == "" {
			g = link
		}
		sum := sha256.Sum256([]byte(g))
		id := hex.EncodeToString(sum[:])
		name := strings.TrimSpace(it.Title)
		if name == "" {
			name = "Untitled"
		}
		out = append(out, domain.CatalogItem{
			ID:           StremioIDPrefix + ":" + id,
			Type:         StremioMetaType,
			Name:         name,
			MagnetURL:    magnet,
			TorrentURL:   torrent,
			InfoHash:     hash,
			Released:     strings.TrimSpace(it.PubDate),
			SubtitlesTag: sub,
		})
	}
	return out, uniqueEraiSlugs(slugAcc), nil
}

// parseFallbackXML handles minimal RSS 2.0 when gofeed fails.
func parseFallbackXML(body []byte) ([]domain.CatalogItem, error) {
	items, _, err := parseFallbackXMLWithSlugs(body)
	return items, err
}
