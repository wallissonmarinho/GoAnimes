package sync

import (
	"net/url"
	"path"
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
	explicitEpisodeStandaloneRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:ep|e)\s?(\d{1,3})\b`)
	numDashRe      = regexp.MustCompile(`\s-\s(\d{1,4})\b`)
	rangeRe        = regexp.MustCompile(`(?i)\b(\d{1,4})\s*(?:~|-)\s*(\d{1,4})\b`)
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
	clean = rangeRe.ReplaceAllString(clean, " ")
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

func ExtractExplicitEpisode(raw string) int {
	clean := explicitEpisodeSubject(raw)
	if clean == "" {
		return 0
	}
	if match := numDashRe.FindStringSubmatch(clean); len(match) > 0 {
		return atoi(match[1])
	}
	if match := seasonEpisodeRe.FindStringSubmatch(clean); len(match) > 0 {
		full := match[0]
		if epMatch := regexp.MustCompile(`(?i)e(\d{1,3})\b`).FindStringSubmatch(full); len(epMatch) > 1 {
			return atoi(epMatch[1])
		}
	}
	if match := explicitEpisodeStandaloneRe.FindStringSubmatch(clean); len(match) > 1 {
		return atoi(match[1])
	}
	if match := epRe.FindStringSubmatch(clean); len(match) > 0 {
		if match[1] != "" {
			return atoi(match[1])
		}
		if len(match) > 2 && match[2] != "" {
			return atoi(match[2])
		}
	}
	return 0
}

func ExtractEpisodeRange(raw string) (int, int) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return 0, 0
	}
	clean = tagRe.ReplaceAllString(clean, " ")
	clean = parenRe.ReplaceAllString(clean, " ")
	clean = braceRe.ReplaceAllString(clean, " ")
	match := rangeRe.FindStringSubmatch(clean)
	if len(match) < 3 {
		return 0, 0
	}
	start := atoi(match[1])
	end := atoi(match[2])
	if start <= 0 || end <= 0 || end < start {
		return 0, 0
	}
	if start == end {
		return 0, 0
	}
	return start, end
}

func explicitEpisodeSubject(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return ""
	}
	if parsed, err := url.Parse(clean); err == nil {
		switch {
		case strings.EqualFold(parsed.Scheme, "magnet"):
			if dn := strings.TrimSpace(parsed.Query().Get("dn")); dn != "" {
				if decoded, decErr := url.QueryUnescape(dn); decErr == nil && strings.TrimSpace(decoded) != "" {
					return decoded
				}
				return dn
			}
			return ""
		case parsed.Scheme == "http" || parsed.Scheme == "https":
			base := strings.TrimSpace(path.Base(parsed.Path))
			if decoded, decErr := url.QueryUnescape(base); decErr == nil && strings.TrimSpace(decoded) != "" {
				return decoded
			}
			return base
		}
	}
	if decoded, err := url.QueryUnescape(clean); err == nil && strings.TrimSpace(decoded) != "" {
		clean = decoded
	}
	return clean
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
