package sync

import (
	"regexp"
	"strings"
)

var (
	spaceRe        = regexp.MustCompile(`\s+`)
	tagRe          = regexp.MustCompile(`\[[^\]]+\]`)
	parenRe        = regexp.MustCompile(`\([^\)]+\)`)
	braceRe        = regexp.MustCompile(`\{[^\}]+\}`)
	seasonEpisodeRe = regexp.MustCompile(`(?i)\bs(\d{1,2})e\d{1,3}\b`)
	seasonWordRe    = regexp.MustCompile(`(?i)\bseason\s+(\d{1,2})\b`)
	shortSeasonRe   = regexp.MustCompile(`(?i)\bs(\d{1,2})\b`)
	epRe           = regexp.MustCompile(`(?i)(?:s\d{1,2})?e(\d{1,3})\b|(?:ep|e)\s?(\d{1,3})\b`)
	numDashRe      = regexp.MustCompile(`\s-\s(\d{1,3})\b`)
	qualityBlockRe = regexp.MustCompile(`(?i)\[([^\]]*\b(?:480p|720p|1080p|2160p)\b[^\]]*)\]`)
	qualityRe      = regexp.MustCompile(`(?i)\b(?:480p|720p|1080p|2160p)\b`)
	noiseTokenRe   = regexp.MustCompile(`(?i)\b(?:web[\s-]?dl|web[\s-]?rip|webrip|bluray|bdrip|hevc|x265|x264|h\.?264|h\.?265|avc|aac\d?(?:\.\d)?|eac3|ddp\d(?:\.\d)?|multi(?:-audio|-subs)?|dual(?:-audio)?|repack|end|finale|uncensored|encoded|more|nf|cr|dsnp|amzn|iq|tver|bili|viki|adn)\b`)
	leadingProviderRe = regexp.MustCompile(`(?i)^(?:erai-raws|erai|toonshub|nekobt)\s+`)
)

func NormalizeTitle(raw string) (nameKey string, episode int, quality string) {
	clean := strings.TrimSpace(raw)
	if match := qualityBlockRe.FindStringSubmatch(raw); len(match) > 1 {
		quality = strings.TrimSpace(match[1])
	} else if match := qualityRe.FindString(raw); match != "" {
		quality = strings.TrimSpace(match)
	}
	clean = tagRe.ReplaceAllString(clean, " ")
	clean = parenRe.ReplaceAllString(clean, " ")
	clean = braceRe.ReplaceAllString(clean, " ")
	if match := epRe.FindStringSubmatch(clean); len(match) > 0 {
		// Group 1: s##e## pattern, Group 2: e## or ep## pattern
		if match[1] != "" {
			episode = atoi(match[1])
		} else if len(match) > 2 && match[2] != "" {
			episode = atoi(match[2])
		}
	} else if match := numDashRe.FindStringSubmatch(clean); len(match) > 0 {
		episode = atoi(match[1])
	}
	clean = qualityRe.ReplaceAllString(clean, " ")
	clean = epRe.ReplaceAllString(clean, " ")
	clean = numDashRe.ReplaceAllString(clean, " ")
	clean = noiseTokenRe.ReplaceAllString(clean, " ")
	clean = leadingProviderRe.ReplaceAllString(clean, " ")
	clean = strings.ToLower(spaceRe.ReplaceAllString(clean, " "))
	clean = strings.TrimSpace(clean)
	nameKey = clean
	return nameKey, episode, quality
}

func ExtractSeasonHint(raw string) int {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return 0
	}
	clean = tagRe.ReplaceAllString(clean, " ")
	clean = parenRe.ReplaceAllString(clean, " ")
	clean = braceRe.ReplaceAllString(clean, " ")
	if match := seasonEpisodeRe.FindStringSubmatch(clean); len(match) > 1 {
		return atoi(match[1])
	}
	if match := seasonWordRe.FindStringSubmatch(clean); len(match) > 1 {
		return atoi(match[1])
	}
	if match := shortSeasonRe.FindStringSubmatch(clean); len(match) > 1 {
		return atoi(match[1])
	}
	return 0
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
