package stremio

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// StremioVideoReleasedISO normalizes release strings for Stremio (ISO 8601).
func StremioVideoReleasedISO(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "1970-01-01T00:00:00.000Z"
	}
	if strings.Contains(s, "T") {
		return s
	}
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s + "T12:00:00.000Z"
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.UnixDate, "Mon, 02 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return "1970-01-01T00:00:00.000Z"
}

// PickStremioVideoReleasedISO prefers a metadata schedule string (e.g. Cinemeta) when set, otherwise RSS/torrent dates.
// Stremio shows the green “upcoming” badge when released is in the future.
func PickStremioVideoReleasedISO(scheduleAirISO, rssFallback string) string {
	if s := strings.TrimSpace(scheduleAirISO); s != "" {
		return StremioVideoReleasedISO(s)
	}
	return StremioVideoReleasedISO(rssFallback)
}

func parseStremioReleasedTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// StremioMetaBehaviorHintsIfScheduled returns behaviorHints like Cinemeta when any video has a future released time.
func StremioMetaBehaviorHintsIfScheduled(videos []map[string]any) map[string]any {
	now := time.Now().UTC()
	for _, v := range videos {
		s, _ := v["released"].(string)
		if t, ok := parseStremioReleasedTime(s); ok && t.After(now) {
			return map[string]any{
				"defaultVideoId":     nil,
				"hasScheduledVideos": true,
			}
		}
	}
	return nil
}

// SortStremioMetaVideosByReleased sorts video rows by "released" ascending.
func SortStremioMetaVideosByReleased(videos []map[string]any) {
	parse := func(s string) time.Time {
		s = strings.TrimSpace(s)
		if s == "" {
			return time.Time{}
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
		return time.Time{}
	}
	sort.SliceStable(videos, func(i, j int) bool {
		si, _ := videos[i]["released"].(string)
		sj, _ := videos[j]["released"].(string)
		ti, tj := parse(si), parse(sj)
		if ti.IsZero() && tj.IsZero() {
			return false
		}
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})
}

// AppendEnrichmentCalendarVideo adds a synthetic video row for a scheduled next episode (calendar metadata).
func AppendEnrichmentCalendarVideo(seriesID string, season, nextEp int, en domain.SeriesEnrichment, groups map[domain.EpSortKey][]domain.CatalogItem, videos *[]map[string]any) {
	if !en.NextAiringFromAniList || en.NextAiringUnix <= 0 || nextEp <= 0 {
		return
	}
	k := domain.EpSortKey{Season: season, Episode: nextEp, Special: false}
	if g, ok := groups[k]; ok && len(g) > 0 {
		return
	}
	vid := domain.EpisodeVideoStremioID(seriesID, season, nextEp, false)
	t := time.Unix(en.NextAiringUnix, 0).UTC()
	released := t.Format(time.RFC3339Nano)
	title := domain.EpisodeListTitle(nextEp, false, en.EpisodeTitleByNum, "")
	if strings.TrimSpace(title) == "" {
		title = "Episódio " + fmt.Sprintf("%d", nextEp)
	}
	title = title + " · agendado"
	row := map[string]any{
		"id":       vid,
		"title":    title,
		"released": released,
		"season":   season,
		"episode":  nextEp,
	}
	if thumb := strings.TrimSpace(en.PosterURL); thumb != "" {
		row["thumbnail"] = thumb
	}
	*videos = append(*videos, row)
}

// SeriesToStremioMetaMaps builds Stremio catalog meta rows (type anime).
func SeriesToStremioMetaMaps(series []domain.CatalogSeries) []map[string]any {
	metas := make([]map[string]any, 0, len(series))
	for _, s := range series {
		genres := []string{"Anime"}
		if len(s.Genres) > 0 {
			genres = append([]string(nil), s.Genres...)
		}
		m := map[string]any{
			"id":     s.ID,
			"type":   "anime",
			"name":   s.Name,
			"genres": genres,
		}
		if s.Poster != "" {
			m["poster"] = s.Poster
		}
		if d := strings.TrimSpace(s.Description); d != "" {
			m["description"] = d
		}
		if ri := strings.TrimSpace(s.ReleaseInfo); ri != "" {
			m["releaseInfo"] = ri
		}
		metas = append(metas, m)
	}
	return metas
}

// StreamFromCatalogItem builds one Stremio stream object or nil if no playable URL/hash.
func StreamFromCatalogItem(it domain.CatalogItem, bingeGroup string, episodeDisplay string) map[string]any {
	label := domain.ShortQualityHint(it.Name)
	if label == "" && strings.Contains(strings.ToUpper(it.Name), "HEVC") {
		label = "HEVC"
	}
	if label == "" {
		label = "Release"
	}
	streamName := "Torrent · " + label
	streamTitle := it.Name
	if ed := strings.TrimSpace(episodeDisplay); ed != "" {
		streamTitle = ed + " — " + it.Name
	}
	if it.InfoHash != "" {
		return map[string]any{
			"name":     streamName,
			"title":    streamTitle,
			"infoHash": it.InfoHash,
			"fileIdx":  0,
			"behaviorHints": map[string]any{
				"bingeGroup": bingeGroup,
			},
		}
	}
	if it.MagnetURL != "" {
		return map[string]any{
			"name":  streamName,
			"title": streamTitle,
			"url":   it.MagnetURL,
		}
	}
	if it.TorrentURL != "" {
		return map[string]any{
			"name":  streamName,
			"title": streamTitle,
			"url":   it.TorrentURL,
		}
	}
	return nil
}

// MergeSeriesEnrichmentIntoStremioMeta merges enrichment into a Stremio meta map (mutates meta).
func MergeSeriesEnrichmentIntoStremioMeta(meta map[string]any, en domain.SeriesEnrichment) {
	if strings.TrimSpace(en.Description) != "" {
		meta["description"] = domain.LocalizeEnrichedDescriptionPTBR(en.Description)
	}
	if len(en.Genres) > 0 {
		meta["genres"] = domain.TranslateAnimeGenresToPTBR(en.Genres)
	}
	if y := strings.TrimSpace(en.SeriesYearLabel); y != "" {
		meta["year"] = y
		meta["releaseInfo"] = y
	} else if en.StartYear > 0 {
		meta["releaseInfo"] = fmt.Sprintf("%d-", en.StartYear)
	}
	if r := strings.TrimSpace(en.SeriesReleasedISO); r != "" {
		meta["released"] = r
	}
	if s := strings.TrimSpace(en.SeriesStatus); s != "" {
		meta["status"] = s
	}
	if id := strings.TrimSpace(en.ImdbID); id != "" {
		meta["imdb_id"] = id
	}
	if en.EpisodeLengthMin > 0 {
		meta["runtime"] = fmt.Sprintf("~%d min por episódio", en.EpisodeLengthMin)
	}
	bg := strings.TrimSpace(en.StremioHeroBackgroundURL)
	if bg == "" {
		bg = strings.TrimSpace(domain.ResolveStremioHeroBackground(en, nil))
	}
	if bg == "" {
		bg = strings.TrimSpace(en.PosterURL)
	}
	if bg == "" {
		if p, ok := meta["poster"].(string); ok {
			bg = strings.TrimSpace(p)
		}
	}
	if bg != "" {
		meta["background"] = bg
	}
	if strings.TrimSpace(en.TrailerYouTubeID) != "" {
		meta["trailers"] = []map[string]any{{"source": en.TrailerYouTubeID, "type": "Trailer"}}
	}
	if tp := strings.TrimSpace(en.TitlePreferred); tp != "" {
		meta["name"] = tp
	}
	if u := strings.TrimSpace(en.PosterURL); u != "" {
		meta["poster"] = u
	}
	if MetaUsesTheTVDBData(en) {
		cur, _ := meta["description"].(string)
		meta["description"] = AppendTheTVDBAttributionToDescription(cur)
	}
}
