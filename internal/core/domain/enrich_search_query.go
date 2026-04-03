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

// AniListSearchQueryCandidates returns search strings to try against AniList (full title first, then shorter fallbacks).
func AniListSearchQueryCandidates(normalized string) []string {
	q := strings.TrimSpace(normalized)
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if len(s) < 3 {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(q)
	// "Main Title: cour / part subtitle" — AniList often indexes the left side only.
	if idx := strings.Index(q, ":"); idx >= 12 && idx < len(q)-4 {
		add(strings.TrimSpace(q[:idx]))
	}
	return out
}
