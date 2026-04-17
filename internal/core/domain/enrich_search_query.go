package domain

import (
	"regexp"
	"strings"
)

var leadingBracketTagRe = regexp.MustCompile(`^\s*\[[^\]]+\]\s*`)

// NormalizeExternalAnimeSearchQuery strips release-style tags and junk so external lookups
// match the same show as RSS-derived series names (e.g. "[Magnet] Title - 01" -> "Title").
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

// ExternalMetadataSearchVersion increments when Cinemeta search / disambiguation logic changes; bump al_search_ver on rows to refetch.
const ExternalMetadataSearchVersion = 7

var externalMetadataSearchIgnoredTokens = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "of": {}, "to": {}, "no": {}, "ni": {},
	"season": {}, "part": {}, "cour": {}, "special": {}, "ova": {}, "tv": {}, "hen": {},
	"2nd": {}, "3rd": {}, "4th": {}, "5th": {}, "6th": {}, "7th": {}, "8th": {}, "9th": {}, "1st": {},
	"torrent": {}, "web": {}, "dl": {}, "aac": {}, "avc": {}, "cr": {},
}

var scoringNonAlnumSplitRe = regexp.MustCompile(`[^a-z0-9]+`)
var scoringBracketTagRe = regexp.MustCompile(`\[[^\]]*\]`)

// ExternalMetadataScoringTokens splits a free-text query into lowercase tokens used to score Cinemeta search hits.
func ExternalMetadataScoringTokens(query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	q = scoringBracketTagRe.ReplaceAllString(q, " ")
	parts := scoringNonAlnumSplitRe.Split(q, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		if _, ok := externalMetadataSearchIgnoredTokens[p]; ok {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// ExternalSearchQueryCandidates returns search strings to try (full title first, then shorter fallbacks).
func ExternalSearchQueryCandidates(normalized string) []string {
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
	// "Main Title: cour / part subtitle" - most providers index the left side only.
	if idx := strings.Index(q, ":"); idx >= 12 && idx < len(q)-4 {
		add(strings.TrimSpace(q[:idx]))
	}
	return out
}
