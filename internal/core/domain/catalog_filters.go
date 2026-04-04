package domain

import (
	"sort"
	"strings"
	"time"
)

// ParseItemReleasedDate parses catalog item Released for filtering (RSS date or RFC3339-style).
func ParseItemReleasedDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	// Date-only when exactly yyyy-mm-dd; longer strings may be RFC3339 (do not truncate — would drop time-of-day).
	if len(raw) == 10 && raw[4] == '-' && raw[7] == '-' {
		if t, err := time.ParseInLocation("2006-01-02", raw, time.UTC); err == nil {
			return t, true
		}
	}
	layouts := []string{time.RFC3339, time.RFC1123Z, time.RFC1123, "2006-01-02T15:04:05Z07:00"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// FilterSeriesWithReleaseSince keeps series that have at least one item released on or after cutoff (UTC midnight).
func FilterSeriesWithReleaseSince(snap *CatalogSnapshot, cutoff time.Time) []CatalogSeries {
	if snap == nil {
		return nil
	}
	cutoff = cutoff.UTC().Truncate(24 * time.Hour)
	want := make(map[string]struct{})
	for _, it := range snap.Items {
		if it.SeriesID == "" {
			continue
		}
		ts, ok := ParseItemReleasedDate(it.Released)
		if !ok {
			continue
		}
		day := ts.UTC().Truncate(24 * time.Hour)
		if !day.Before(cutoff) {
			want[it.SeriesID] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil
	}
	out := make([]CatalogSeries, 0, len(want))
	for _, s := range snap.Series {
		if _, ok := want[s.ID]; ok {
			out = append(out, s)
		}
	}
	return out
}

// FilterSeriesWithRecentReleases keeps series with any episode released in the last `days` calendar days (including today).
func FilterSeriesWithRecentReleases(snap *CatalogSnapshot, days int) []CatalogSeries {
	if days < 1 {
		days = 7
	}
	cutoff := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -days)
	return FilterSeriesWithReleaseSince(snap, cutoff)
}

// UniqueGenreLabelsFromCatalogSeries returns distinct genre labels from all series, sorted (pt-BR when mapeado).
// Each label is passed through TranslateAnimeGenresToPTBR so inglês do catálogo/BD e pt-BR deduplicam (ex. Action + Ação → Ação).
func UniqueGenreLabelsFromCatalogSeries(series []CatalogSeries) []string {
	seen := make(map[string]struct{})
	for _, s := range series {
		for _, g := range s.Genres {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			pt := TranslateAnimeGenresToPTBR([]string{g})
			if len(pt) == 0 {
				continue
			}
			seen[pt[0]] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// genreMatchPTBR reports whether a catalog genre label matches the filter (English/pt-BR, case-insensitive).
func genreMatchPTBR(catalogGenre, want string) bool {
	want = strings.TrimSpace(want)
	g := strings.TrimSpace(catalogGenre)
	if want == "" || g == "" {
		return false
	}
	wNorm := TranslateAnimeGenresToPTBR([]string{want})
	gNorm := TranslateAnimeGenresToPTBR([]string{g})
	var wLab, gLab string
	if len(wNorm) > 0 {
		wLab = wNorm[0]
	} else {
		wLab = want
	}
	if len(gNorm) > 0 {
		gLab = gNorm[0]
	} else {
		gLab = g
	}
	if strings.EqualFold(wLab, gLab) {
		return true
	}
	return strings.EqualFold(want, g)
}

// FilterSeriesByGenre keeps series whose Genres match want (labels normalized with TranslateAnimeGenresToPTBR).
func FilterSeriesByGenre(series []CatalogSeries, want string) []CatalogSeries {
	want = strings.TrimSpace(want)
	if want == "" {
		return series
	}
	out := make([]CatalogSeries, 0, len(series))
outer:
	for _, s := range series {
		for _, g := range s.Genres {
			if genreMatchPTBR(g, want) {
				out = append(out, s)
				continue outer
			}
		}
	}
	return out
}
