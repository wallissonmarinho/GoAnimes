package anilist

import (
	"regexp"
	"strings"
)

var htmlTagRe = regexp.MustCompile(`(?s)<[^>]+>`)

// NormalizeDescription turns AniList HTML-ish / markup into plain text for Stremio.
func NormalizeDescription(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Join(strings.Fields(s), " ")
	const max = 2000
	r := []rune(s)
	if len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
