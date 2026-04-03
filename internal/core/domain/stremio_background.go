package domain

import (
	"math"
	"regexp"
	"strings"
)

// Stremio meta background: wide hero (~16:9) works best; TMDB provides exact WxH.
// See https://github.com/Stremio/stremio-addon-sdk/blob/master/docs/api/responses/meta.md
const (
	stremioBackdropTargetW = 1280
	stremioBackdropTargetH = 720
)

// BackgroundCandidate is one image URL with pixel size for scoring (TMDB images include width/height).
type BackgroundCandidate struct {
	URL string
	W   int
	H   int
}

var imdbTTInURL = regexp.MustCompile(`(?i)imdb\.com/title/(tt\d+)`)

// NormalizeIMDbID returns tt1234567 from a bare id or IMDb URL, or "" if none.
func NormalizeIMDbID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := imdbTTInURL.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "tt") && len(s) >= 3 {
		return s
	}
	return ""
}

// PickBestStremioBackground chooses the URL whose aspect ratio and size are closest to a 1280×720 hero.
// Falls back to posterFallback when no candidate has positive dimensions.
func PickBestStremioBackground(cands []BackgroundCandidate, posterFallback string) string {
	targetAR := float64(stremioBackdropTargetW) / float64(stremioBackdropTargetH)
	bestURL := ""
	bestScore := 1e9
	for _, c := range cands {
		u := strings.TrimSpace(c.URL)
		if u == "" || c.W <= 0 || c.H <= 0 {
			continue
		}
		ar := float64(c.W) / float64(c.H)
		score := math.Abs(ar-targetAR) / targetAR
		score += 0.12 * math.Abs(float64(c.W-stremioBackdropTargetW)) / float64(stremioBackdropTargetW)
		score += 0.08 * math.Abs(float64(c.H-stremioBackdropTargetH)) / float64(stremioBackdropTargetH)
		if score < bestScore {
			bestScore, bestURL = score, u
		}
	}
	if bestURL != "" {
		return bestURL
	}
	return strings.TrimSpace(posterFallback)
}

// EnrichmentBackgroundCandidates builds nominal WxH guesses for AniList/Kitsu URLs so we can compare
// them to TMDB backdrops without fetching every image.
func EnrichmentBackgroundCandidates(en AniListSeriesEnrichment) []BackgroundCandidate {
	seen := make(map[string]struct{})
	var out []BackgroundCandidate
	add := func(u string, w, h int) {
		u = strings.TrimSpace(u)
		if u == "" || w <= 0 || h <= 0 {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, BackgroundCandidate{URL: u, W: w, H: h})
	}
	if b := strings.TrimSpace(en.AniListBannerURL); b != "" {
		add(b, 2048, 858)
	}
	bg := strings.TrimSpace(en.BackgroundURL)
	ban := strings.TrimSpace(en.AniListBannerURL)
	if bg != "" && bg != ban {
		if strings.Contains(strings.ToLower(bg), "kitsu") {
			add(bg, 1920, 1080)
		} else {
			add(bg, 1920, 800)
		}
	}
	return out
}

// ResolveStremioHeroBackground merges AniList/Kitsu nominal candidates with TMDB (exact sizes) and picks the best.
func ResolveStremioHeroBackground(en AniListSeriesEnrichment, tmdb []BackgroundCandidate) string {
	c := EnrichmentBackgroundCandidates(en)
	c = append(c, tmdb...)
	return PickBestStremioBackground(c, en.PosterURL)
}
