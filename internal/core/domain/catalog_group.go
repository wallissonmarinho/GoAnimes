package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// IsEpisodeVideoStremioID is true for grouped episode rows (one list entry, multiple streams on play).
func IsEpisodeVideoStremioID(id string) bool {
	return strings.HasPrefix(id, stremioPrefix+":vid:")
}

// EpisodeVideoStremioID is stable per series + season + episode (+ special); all qualities share it.
func EpisodeVideoStremioID(seriesID string, season, episode int, isSpecial bool) string {
	sp := "0"
	if isSpecial {
		sp = "1"
	}
	h := seriesID + "|" + strconv.Itoa(season) + "|" + strconv.Itoa(episode) + "|" + sp
	sum := sha256.Sum256([]byte(h))
	return stremioPrefix + ":vid:" + hex.EncodeToString(sum[:8])
}

// epSortKey groups catalog items into one Stremio video row.
type epSortKey struct {
	Season  int
	Episode int
	Special bool
}

// GroupItemsByEpisode buckets releases for one series (SD/720/1080 → same bucket).
func GroupItemsByEpisode(items []CatalogItem, seriesID string) map[epSortKey][]CatalogItem {
	m := make(map[epSortKey][]CatalogItem)
	for _, it := range items {
		if it.SeriesID != seriesID {
			continue
		}
		k := epSortKey{Season: it.Season, Episode: it.Episode, Special: it.IsSpecial}
		m[k] = append(m[k], it)
	}
	return m
}

// OrderedEpisodeKeys sorts group keys for the meta videos list.
func OrderedEpisodeKeys(m map[epSortKey][]CatalogItem) []epSortKey {
	keys := make([]epSortKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		if a.Special != b.Special {
			return !a.Special
		}
		return a.Episode < b.Episode
	})
	return keys
}

// LatestReleased picks the newest RSS date string in the group (yyyy-mm-dd lex sort works).
func LatestReleased(items []CatalogItem) string {
	var best string
	for _, it := range items {
		if strings.TrimSpace(it.Released) > best {
			best = it.Released
		}
	}
	return best
}

var streamingEpTitleRe = regexp.MustCompile(`(?i)^Episode\s+(\d+)\s*(?:-|–|—)\s*(.+)$`)

// EpisodeTitlesFromStreamingList builds episode number → title from AniList streamingEpisodes titles
// (e.g. "Episode 5 - Phantoms of the Dead").
func EpisodeTitlesFromStreamingList(rawTitles []string) map[int]string {
	out := make(map[int]string)
	for _, raw := range rawTitles {
		t := strings.TrimSpace(raw)
		m := streamingEpTitleRe.FindStringSubmatch(t)
		if len(m) != 3 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			continue
		}
		name := strings.TrimSpace(m[2])
		if name == "" {
			continue
		}
		if _, ok := out[n]; !ok {
			out[n] = name
		}
	}
	return out
}

// eraiEpisodeTailRe captures the part of an Erai-style release title after the episode number (codec, tags).
var eraiEpisodeTailRe = regexp.MustCompile(`(?i)-\s*(?:\d{1,4}(?:v\d+)?|Special)\s+(.*)$`)

// TorrentReleaseEpisodeSuffix returns a short label from the torrent filename when AniList has no streaming episode title.
func TorrentReleaseEpisodeSuffix(releaseTitle string) string {
	s := strings.TrimSpace(releaseTitle)
	if s == "" {
		return ""
	}
	if m := eraiEpisodeTailRe.FindStringSubmatch(s); len(m) > 1 {
		tail := strings.TrimSpace(m[1])
		tail = strings.Join(strings.Fields(tail), " ")
		if r := []rune(tail); len(r) > 88 {
			tail = string(r[:85]) + "…"
		}
		if tail != "" {
			return tail
		}
	}
	var parts []string
	if strings.Contains(strings.ToUpper(s), "HEVC") {
		parts = append(parts, "HEVC")
	}
	if q := ShortQualityHint(s); q != "" {
		parts = append(parts, q)
	}
	return strings.Join(parts, " · ")
}

// EpisodeListTitle is the Stremio row label without quality (qualities show as stream choices).
// epTitles is optional AniList/Jikan episode titles keyed by episode number (season 1 assumed).
// releaseHint is optional legacy text when epTitles is empty; Stremio list rows omit it so codec tags
// from Erai titles do not replace human episode titles (quality stays on stream entries only).
func EpisodeListTitle(episode int, isSpecial bool, epTitles map[int]string, releaseHint string) string {
	if isSpecial {
		return "Especial"
	}
	base := "Episódio " + strconv.Itoa(episode)
	if epTitles != nil {
		if t, ok := epTitles[episode]; ok {
			t = strings.TrimSpace(t)
			if t != "" {
				return base + " · " + t
			}
		}
	}
	if h := strings.TrimSpace(releaseHint); h != "" {
		return base + " · " + h
	}
	return base
}

// EpisodeListTitleForGroup builds the Stremio episode row title (AniList/Jikan when present, else Episódio N only).
func EpisodeListTitleForGroup(episode int, special bool, epTitles map[int]string, group []CatalogItem) string {
	_ = group
	return EpisodeListTitle(episode, special, epTitles, "")
}

// StreamQualityRank higher = preferred default ordering in the stream picker.
func StreamQualityRank(it CatalogItem) int {
	if strings.Contains(strings.ToUpper(it.Name), "1080P") {
		return 100
	}
	if strings.Contains(strings.ToUpper(it.Name), "720P") {
		return 80
	}
	if q := ShortQualityHint(it.Name); q == "SD" {
		return 60
	}
	if strings.Contains(strings.ToUpper(it.Name), "HEVC") {
		return 50
	}
	if q := ShortQualityHint(it.Name); q != "" {
		return 40
	}
	return 30
}

// SortItemsForStreamChoices orders variants for the stream list (best quality first).
func SortItemsForStreamChoices(items []CatalogItem) {
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := StreamQualityRank(items[i]), StreamQualityRank(items[j])
		if ri != rj {
			return ri > rj
		}
		return items[i].Name < items[j].Name
	})
}

// ItemsForEpisodeVideoID returns all releases for one logical episode (any quality).
func ItemsForEpisodeVideoID(items []CatalogItem, videoID string) []CatalogItem {
	var out []CatalogItem
	for _, it := range items {
		if it.SeriesID == "" {
			continue
		}
		if EpisodeVideoStremioID(it.SeriesID, it.Season, it.Episode, it.IsSpecial) == videoID {
			out = append(out, it)
		}
	}
	SortItemsForStreamChoices(out)
	return out
}

// AniListSearchQueryFromItems returns the RSS-parsed series title (best match for AniList/Jikan search).
// Display name may be native Japanese from enrichment; items keep the Erai string used for lookup.
func AniListSearchQueryFromItems(items []CatalogItem, seriesID string) string {
	for _, it := range items {
		if it.SeriesID != seriesID {
			continue
		}
		if q := strings.TrimSpace(it.SeriesName); q != "" {
			return q
		}
	}
	return ""
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

// Erai per-anime feeds repeat each release as [Torrent] and [Magnet]; both prefixes must strip
// or the same show splits into two Stremio series and episodes look "missing" on one of them.
var eraiReleaseTitleRe = regexp.MustCompile(`(?i)^(?:\[(?:torrent|magnet)\]\s*)?(.+?)\s*-\s*(?:(\d{1,4})(?:v\d+)?|(Special))\b`)

var eraiSeasonSuffixRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\s+(?:(\d{1,2})(?:st|nd|rd|th)\s+Season)\s*$`),
	regexp.MustCompile(`(?i)\s+Season\s+(\d{1,2})\s*$`),
	regexp.MustCompile(`(?i)\s+S(\d{1,2})\s*$`),
	regexp.MustCompile(`(?i)\s+Part\s+(\d{1,2})\s*$`),
	regexp.MustCompile(`(?i)\s+Cour\s+(\d{1,2})\s*$`),
}

// ParseEraiReleaseTitle parses Erai-style RSS titles into series name and episode.
func ParseEraiReleaseTitle(title string) (seriesName string, episode int, isSpecial, ok bool) {
	title = strings.TrimSpace(title)
	m := eraiReleaseTitleRe.FindStringSubmatch(title)
	if len(m) != 4 {
		return "", 0, false, false
	}
	seriesName = strings.TrimSpace(m[1])
	if seriesName == "" {
		return "", 0, false, false
	}
	if strings.EqualFold(strings.TrimSpace(m[3]), "Special") {
		return seriesName, 0, true, true
	}
	if m[2] == "" {
		return "", 0, false, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n < 0 {
		return "", 0, false, false
	}
	return seriesName, n, false, true
}

// EraiSeasonFromSeriesName returns Stremio season (1 if no suffix) from the series segment of an Erai title.
func EraiSeasonFromSeriesName(seriesPart string) (season int) {
	seriesPart = strings.TrimSpace(seriesPart)
	if seriesPart == "" {
		return 1
	}
	for _, re := range eraiSeasonSuffixRes {
		sub := re.FindStringSubmatch(seriesPart)
		if len(sub) < 2 {
			continue
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 1 || n > 99 {
			continue
		}
		return n
	}
	return 1
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
		season := 1
		if ok && sn != "" {
			season = EraiSeasonFromSeriesName(sn)
		}
		it.SeriesName = sn
		it.SeriesID = SeriesStremioID(sn)
		it.Season = season
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

// ApplyAniListEnrichmentToSeries copies cached AniList/Jikan fields onto catalog rows (Discover list).
func ApplyAniListEnrichmentToSeries(snap *CatalogSnapshot) {
	if snap == nil || len(snap.Series) == 0 || len(snap.AniListBySeries) == 0 {
		return
	}
	for i := range snap.Series {
		if en, ok := snap.AniListBySeries[snap.Series[i].ID]; ok {
			ApplyEnrichmentToCatalogSeries(&snap.Series[i], en)
		}
	}
}

// ApplyEnrichmentToCatalogSeries merges cached AniList/Jikan metadata into one catalog row.
func ApplyEnrichmentToCatalogSeries(s *CatalogSeries, en AniListSeriesEnrichment) {
	if s == nil {
		return
	}
	if u := strings.TrimSpace(en.PosterURL); u != "" {
		s.Poster = u
	}
	if t := strings.TrimSpace(en.TitlePreferred); t != "" && !ContainsJapaneseScript(t) {
		s.Name = t
	}
	if d := strings.TrimSpace(en.Description); d != "" {
		s.Description = LocalizeAniListDescriptionPTBR(d)
	}
	if len(en.Genres) > 0 {
		s.Genres = TranslateAnimeGenresToPTBR(append([]string(nil), en.Genres...))
	}
	if en.StartYear > 0 {
		s.ReleaseInfo = fmt.Sprintf("%d-", en.StartYear)
	}
}

// EnsureSnapshotGrouped re-derives series/season/episode from each item Name and rebuilds Series.
func EnsureSnapshotGrouped(snap *CatalogSnapshot) {
	if snap == nil {
		return
	}
	if len(snap.Items) == 0 {
		snap.Series = nil
		return
	}
	if len(snap.Items) > 0 {
		AssignSeriesFields(snap.Items)
	}
	snap.Series = BuildSeriesList(snap.Items)
}

// MergeCatalogItemsByID keeps the previous catalog and layers the latest RSS fetch on top.
// Same Stremio item ID (stable from RSS guid/link) is replaced so magnets/info_hash can refresh;
// entries that dropped off the feed window stay until you clear the DB or remove the app data.
func MergeCatalogItemsByID(prev, incoming []CatalogItem) []CatalogItem {
	by := make(map[string]CatalogItem, len(prev)+len(incoming))
	for _, it := range prev {
		if id := strings.TrimSpace(it.ID); id != "" {
			by[id] = it
		}
	}
	for _, it := range incoming {
		if id := strings.TrimSpace(it.ID); id != "" {
			by[id] = it
		}
	}
	out := make([]CatalogItem, 0, len(by))
	for _, it := range by {
		out = append(out, it)
	}
	return out
}

// SortCatalogItemsInPlace orders items after AssignSeriesFields for stable persistence and diffs.
func SortCatalogItemsInPlace(items []CatalogItem) {
	if len(items) < 2 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		sa, sb := strings.ToLower(a.SeriesID), strings.ToLower(b.SeriesID)
		if sa != sb {
			return sa < sb
		}
		if a.Season != b.Season {
			return a.Season < b.Season
		}
		if a.IsSpecial != b.IsSpecial {
			return !a.IsSpecial
		}
		if a.Episode != b.Episode {
			return a.Episode < b.Episode
		}
		if a.Released != b.Released {
			return a.Released > b.Released
		}
		return a.ID < b.ID
	})
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

