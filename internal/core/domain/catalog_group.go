package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const stremioPrefix = "goanimes"

// IsSeriesStremioID is true when id refers to a grouped show (catalog / meta), not a single release.
func IsSeriesStremioID(id string) bool {
	return strings.HasPrefix(id, stremioPrefix+":series:")
}

// SeriesStremioID returns a stable Stremio id for a series (catalog + meta).
func SeriesStremioID(seriesName string) string {
	sum := sha256.Sum256([]byte(normalizeSeriesKey(seriesName)))
	return stremioPrefix + ":series:" + hex.EncodeToString(sum[:8])
}

func normalizeSeriesKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}

var eraiReleaseTitleRe = regexp.MustCompile(`(?i)^(?:\[torrent\]\s*)?(.+?)\s*-\s*(\d{1,4}|Special)\b`)

// ParseEraiReleaseTitle parses Erai-style RSS titles into series name and episode.
func ParseEraiReleaseTitle(title string) (seriesName string, episode int, isSpecial, ok bool) {
	title = strings.TrimSpace(title)
	m := eraiReleaseTitleRe.FindStringSubmatch(title)
	if len(m) != 3 {
		return "", 0, false, false
	}
	seriesName = strings.TrimSpace(m[1])
	if seriesName == "" {
		return "", 0, false, false
	}
	switch strings.ToLower(m[2]) {
	case "special":
		return seriesName, 0, true, true
	default:
		n, err := strconv.Atoi(m[2])
		if err != nil || n < 0 {
			return "", 0, false, false
		}
		return seriesName, n, false, true
	}
}

var qualityHintRe = regexp.MustCompile(`\[(720p|1080p|SD)\b`)

// ShortQualityHint extracts a short quality label from the release title.
func ShortQualityHint(fullTitle string) string {
	if m := qualityHintRe.FindStringSubmatch(fullTitle); len(m) > 1 {
		return m[1]
	}
	if strings.Contains(strings.ToUpper(fullTitle), "HEVC") {
		return "HEVC"
	}
	return ""
}

// AssignSeriesFields sets series / episode fields on each catalog item from its Name.
func AssignSeriesFields(items []CatalogItem) {
	for i := range items {
		it := &items[i]
		sn, ep, isSp, ok := ParseEraiReleaseTitle(it.Name)
		if !ok || sn == "" {
			sn = strings.TrimSpace(it.Name)
			if sn == "" {
				sn = "Unknown"
			}
			if len(sn) > 100 {
				sn = sn[:100] + "…"
			}
			ep = 1
			isSp = false
		}
		it.SeriesName = sn
		it.SeriesID = SeriesStremioID(sn)
		it.Season = 1
		it.Episode = ep
		it.IsSpecial = isSp
	}
}

// BuildSeriesList returns one CatalogSeries per distinct SeriesID (sorted by name).
func BuildSeriesList(items []CatalogItem) []CatalogSeries {
	seen := make(map[string]CatalogSeries)
	for _, it := range items {
		if it.SeriesID == "" {
			continue
		}
		if _, ok := seen[it.SeriesID]; ok {
			continue
		}
		seen[it.SeriesID] = CatalogSeries{
			ID:     it.SeriesID,
			Name:   it.SeriesName,
			Poster: SeriesPosterURL(it.SeriesName),
		}
	}
	out := make([]CatalogSeries, 0, len(seen))
	for _, s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// SeriesPosterURL is a deterministic placeholder poster (no external anime DB required).
func SeriesPosterURL(seriesName string) string {
	label := seriesName
	r := []rune(label)
	if len(r) > 22 {
		label = string(r[:22]) + "…"
	}
	return "https://placehold.co/480x720/2a2a35/eeeeee/png?text=" + url.QueryEscape(label)
}

func seriesFieldsMissing(items []CatalogItem) bool {
	for _, it := range items {
		if it.SeriesID == "" {
			return true
		}
	}
	return false
}

// EnsureSnapshotGrouped fills per-item series fields when missing and rebuilds Series.
func EnsureSnapshotGrouped(snap *CatalogSnapshot) {
	if snap == nil {
		return
	}
	if len(snap.Items) == 0 {
		snap.Series = nil
		return
	}
	if seriesFieldsMissing(snap.Items) {
		AssignSeriesFields(snap.Items)
	}
	snap.Series = BuildSeriesList(snap.Items)
}

// SortEpisodes returns a copy of items belonging to seriesID, sorted for Stremio videos.
func SortEpisodes(items []CatalogItem, seriesID string) []CatalogItem {
	var out []CatalogItem
	for _, it := range items {
		if it.SeriesID == seriesID {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		if a.IsSpecial != b.IsSpecial {
			return !a.IsSpecial
		}
		if a.Episode != b.Episode {
			return a.Episode < b.Episode
		}
		return a.Released > b.Released
	})
	return out
}

// EpisodeVideoTitle builds the Stremio video row title.
func EpisodeVideoTitle(it CatalogItem) string {
	q := ShortQualityHint(it.Name)
	if it.IsSpecial {
		if q != "" {
			return "Special · " + q
		}
		return "Special"
	}
	if q != "" {
		return "E" + strconv.Itoa(it.Episode) + " · " + q
	}
	return "E" + strconv.Itoa(it.Episode)
}
