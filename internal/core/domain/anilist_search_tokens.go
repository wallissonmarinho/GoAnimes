package domain

import (
	"regexp"
	"strings"
)

// AniListSearcherVersion increments when AniList search / disambiguation logic changes; older cached rows refetch once.
const AniListSearcherVersion = 6

// Tokens ignored when scoring AniList search results (too generic or noisy in RSS titles).
var animeSearchIgnoredTokens = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "of": {}, "to": {}, "no": {}, "ni": {},
	"season": {}, "part": {}, "cour": {}, "special": {}, "ova": {}, "tv": {}, "hen": {},
	"2nd": {}, "3rd": {}, "4th": {}, "5th": {}, "6th": {}, "7th": {}, "8th": {}, "9th": {}, "1st": {},
	"torrent": {}, "web": {}, "dl": {}, "aac": {}, "avc": {}, "cr": {},
}

var nonAlnumSplitRe = regexp.MustCompile(`[^a-z0-9]+`)
var bracketTagRe = regexp.MustCompile(`\[[^\]]*\]`)

// AnimeSearchScoringTokens splits a free-text query into lowercase tokens used to pick the best AniList hit.
func AnimeSearchScoringTokens(query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	q = bracketTagRe.ReplaceAllString(q, " ")
	parts := nonAlnumSplitRe.Split(q, -1)
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		if _, ok := animeSearchIgnoredTokens[p]; ok {
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
