package rss

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/mmcdole/gofeed"
)

var eraiAnimeListSlugRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)https?://(?:www\.)?erai-raws\.info/anime-list/([a-z0-9][-a-z0-9.]*)/`),
	regexp.MustCompile(`(?i)https?://erai\.to/anime-list/([a-z0-9][-a-z0-9.]*)/`),
	regexp.MustCompile(`(?i)/anime-list/([a-z0-9][-a-z0-9.]*)/`),
}

// eraiEpisodePageSlugRes matches /episodes/{slug}/ on Erai (global RSS description links).
var eraiEpisodePageSlugRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)https?://(?:www\.)?erai-raws\.info/episodes/([a-z0-9][-a-z0-9.]*)/`),
	regexp.MustCompile(`(?i)/episodes/([a-z0-9][-a-z0-9.]*)/`),
}

var (
	eraiEpTailAudioRe   = regexp.MustCompile(`(?i)-(chinese|japanese)-audio$`)
	eraiEpTailMultiRe   = regexp.MustCompile(`(?i)-multi$`)
	eraiEpTailVNumRe    = regexp.MustCompile(`-\d+v\d+$`)
	eraiEpTailRevRe     = regexp.MustCompile(`-\d+-\d$`) // e.g. darwin-jihen-13-3
	eraiEpTailEp2PlusRe = regexp.MustCompile(`-\d{2,}$`) // 01, 12, 13; avoids eating -3 from ...-part-3
)

const eraiAnimeSlugMaxLen = 192

// EraiSourceOriginAndToken returns scheme+host and token query for a registered Erai RSS feed URL.
// Per-anime feed URLs must use this token (from the configured RSS URL), never from item HTML/links.
func EraiSourceOriginAndToken(feedURL string) (origin string, token string) {
	u, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil || u.Host == "" {
		return "", ""
	}
	host := strings.ToLower(u.Host)
	if !strings.Contains(host, "erai-raws") && !strings.Contains(host, "erai.to") {
		return "", ""
	}
	token = strings.TrimSpace(u.Query().Get("token"))
	u.Path, u.RawQuery, u.Fragment = "", "", ""
	origin = strings.TrimRight(u.String(), "/")
	return origin, token
}

// BuildEraiPerAnimeFeedURL builds …/anime-list/{slug}/feed/?token=… using origin + token from configured sources.
func BuildEraiPerAnimeFeedURL(origin, slug, token string) string {
	slug = strings.TrimSpace(slug)
	if origin == "" || slug == "" || token == "" {
		return ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/anime-list/" + slug + "/feed/"
	q := url.Values{}
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

// ExtractEraiAnimeListSlugs finds anime-list path segments inside HTML, URLs, or RSS blocks.
func ExtractEraiAnimeListSlugs(haystacks ...string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, h := range haystacks {
		for _, re := range eraiAnimeListSlugRes {
			for _, m := range re.FindAllStringSubmatch(h, -1) {
				if len(m) < 2 {
					continue
				}
				s := strings.TrimSpace(strings.ToLower(m[1]))
				if s == "" || s == "feed" || len(s) > eraiAnimeSlugMaxLen {
					continue
				}
				if _, ok := seen[s]; ok {
					continue
				}
				seen[s] = struct{}{}
				out = append(out, s)
			}
		}
	}
	return out
}

// EraiAnimeListSlugFromEpisodeSlug turns an /episodes/… path segment into the anime-list slug by
// stripping trailing episode markers (-01, -05v2, -chinese-audio, etc.).
func EraiAnimeListSlugFromEpisodeSlug(episodeSeg string) string {
	s := strings.TrimSpace(strings.ToLower(episodeSeg))
	if s == "" || s == "feed" {
		return ""
	}
	const maxIter = 64
	for i := 0; i < maxIter; i++ {
		prev := s
		if eraiEpTailAudioRe.MatchString(s) {
			s = eraiEpTailAudioRe.ReplaceAllString(s, "")
			continue
		}
		if eraiEpTailMultiRe.MatchString(s) {
			s = eraiEpTailMultiRe.ReplaceAllString(s, "")
			continue
		}
		if eraiEpTailVNumRe.MatchString(s) {
			s = eraiEpTailVNumRe.ReplaceAllString(s, "")
			continue
		}
		if eraiEpTailRevRe.MatchString(s) {
			if j := strings.LastIndex(s, "-"); j > 0 {
				s = s[:j]
			}
			continue
		}
		if s == prev {
			break
		}
	}
	// One pass only: episode numbers are usually 2+ digits (01, 12). A second pass would turn
	// yami-shibai-16-12 into yami-shibai by also stripping the season number 16.
	if eraiEpTailEp2PlusRe.MatchString(s) {
		s = eraiEpTailEp2PlusRe.ReplaceAllString(s, "")
	}
	if s == "" || len(s) > eraiAnimeSlugMaxLen {
		return ""
	}
	return s
}

// ExtractEraiAnimeListSlugsFromEpisodeLinks finds erai-raws.info/episodes/… URLs and derives anime-list slugs.
func ExtractEraiAnimeListSlugsFromEpisodeLinks(haystacks ...string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, h := range haystacks {
		for _, re := range eraiEpisodePageSlugRes {
			for _, m := range re.FindAllStringSubmatch(h, -1) {
				if len(m) < 2 {
					continue
				}
				slug := EraiAnimeListSlugFromEpisodeSlug(m[1])
				if slug == "" {
					continue
				}
				if _, ok := seen[slug]; ok {
					continue
				}
				seen[slug] = struct{}{}
				out = append(out, slug)
			}
		}
	}
	return out
}

func discoverSlugsFromGofeedItem(raw string, item *gofeed.Item) []string {
	if item == nil {
		return nil
	}
	block := itemXMLBlock(raw, item)
	var hay []string
	hay = append(hay, block, item.Link, item.GUID, item.Description, item.Content)
	for _, c := range item.Categories {
		hay = append(hay, c)
	}
	var slugs []string
	slugs = append(slugs, ExtractEraiAnimeListSlugs(hay...)...)
	slugs = append(slugs, ExtractEraiAnimeListSlugsFromEpisodeLinks(hay...)...)
	return slugs
}

func uniqueEraiSlugs(in []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
