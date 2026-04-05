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

// AppendAniListCalendarVideo adds a synthetic video row for AniList-scheduled next episode.
func AppendAniListCalendarVideo(seriesID string, season, nextEp int, en domain.AniListSeriesEnrichment, groups map[domain.EpSortKey][]domain.CatalogItem, videos *[]map[string]any) {
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
	title = title + " · agendado (AniList)"
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

// MergeAniListIntoStremioSeriesMeta merges enrichment into a Stremio meta map (mutates meta).
func MergeAniListIntoStremioSeriesMeta(meta map[string]any, en domain.AniListSeriesEnrichment) {
	if strings.TrimSpace(en.Description) != "" {
		meta["description"] = domain.LocalizeAniListDescriptionPTBR(en.Description)
	}
	if len(en.Genres) > 0 {
		meta["genres"] = domain.TranslateAnimeGenresToPTBR(en.Genres)
	}
	if en.StartYear > 0 {
		meta["releaseInfo"] = fmt.Sprintf("%d-", en.StartYear)
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
}
