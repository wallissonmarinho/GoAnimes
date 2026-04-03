package domain

import (
	"regexp"
	"strings"
)

var leadingBracketTagRe = regexp.MustCompile(`^\s*\[[^\]]+\]\s*`)

// NormalizeExternalAnimeSearchQuery strips release-style tags and junk so AniList/Jikan
// match the same show as RSS-derived series names (e.g. "[Magnet] Title - 01" → "Title").
func NormalizeExternalAnimeSearchQuery(title string) string {
	s := strings.TrimSpace(title)
	for i := 0; i < 12; i++ {
		next := leadingBracketTagRe.ReplaceAllString(s, "")
		next = strings.TrimSpace(next)
		if next == s {
			break
		}
		s = next
	}
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\x22`, `"`)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
