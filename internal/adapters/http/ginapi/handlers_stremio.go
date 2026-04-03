package ginapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

const (
	catalogStremioID     = "goanimes"
	catalogStremioWeekID = "goanimes-week"
	stremioTypeAnime     = "anime"
	stremioTypeMovie     = "movie"
	stremioTypeSeries    = "series"
	// stremioManifestVersion: PATCH = fixes, tuning, deps, docs; MINOR = nova funcionalidade visível (API, sync, catálogo Stremio); MAJOR = contrato que parte instalações.
	stremioManifestVersion = "1.5.5"
)

func stremioMetaOrStreamTypeOK(t string) bool {
	switch t {
	case stremioTypeAnime, stremioTypeMovie, "series":
		return true
	default:
		return false
	}
}

// Stremio requires each series video object to include "released" as ISO 8601; missing → "no metadata" in the app.
func stremioVideoReleasedISO(raw string) string {
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

func sortStremioMetaVideosByReleased(videos []gin.H) {
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

// appendAniListCalendarVideo adds a synthetic Stremio video row when AniList reports a next episode
// air time but the RSS catalog has no release for that episode yet (Stremio Calendar).
func appendAniListCalendarVideo(seriesID string, season, nextEp int, en domain.AniListSeriesEnrichment, groups map[domain.EpSortKey][]domain.CatalogItem, videos *[]gin.H) {
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
	row := gin.H{
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

func stremioUnescapePathParam(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

func parseStremioCatalogPathParam(catalogPath string) (catalogID string, extras map[string]string, ok bool) {
	p := strings.TrimPrefix(strings.TrimSpace(catalogPath), "/")
	if p == "" {
		return "", nil, false
	}
	p = strings.TrimSuffix(p, ".json")
	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, false
	}
	catalogID = parts[0]
	extras = make(map[string]string)
	for _, seg := range parts[1:] {
		k, v, has := strings.Cut(seg, "=")
		if !has {
			continue
		}
		key, errK := url.PathUnescape(strings.TrimSpace(k))
		val, errV := url.PathUnescape(strings.TrimSpace(v))
		if errK != nil || errV != nil || key == "" {
			continue
		}
		extras[key] = val
	}
	return catalogID, extras, true
}

func seriesToStremioMetas(series []domain.CatalogSeries) []gin.H {
	metas := make([]gin.H, 0, len(series))
	for _, s := range series {
		genres := []string{"Anime"}
		if len(s.Genres) > 0 {
			genres = append([]string(nil), s.Genres...)
		}
		m := gin.H{
			"id":     s.ID,
			"type":   stremioTypeAnime,
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

func (h *handlers) getManifest(c *gin.Context) {
	snap := h.deps.Catalog.Snapshot()
	genreOpts := domain.UniqueGenreLabelsFromCatalogSeries(snap.Series)
	genreExtra := []gin.H{{
		"name":       "genre",
		"isRequired": false,
		"options":    genreOpts,
	}}
	genres := genreOpts
	c.JSON(http.StatusOK, gin.H{
		"id":          "org.goanimes",
		"version":     stremioManifestVersion,
		"name":        "GoAnimes",
		"description": "RSS anime torrents with pt-BR (Erai [br]) filter",
		"types":       []string{stremioTypeAnime, stremioTypeMovie, stremioTypeSeries},
		"genres":      genres,
		"catalogs": []gin.H{
			{"type": stremioTypeAnime, "id": catalogStremioID, "name": "GoAnimes", "extra": genreExtra},
			{"type": stremioTypeAnime, "id": catalogStremioWeekID, "name": "GoAnimes · Últimos 7 dias", "extra": genreExtra},
		},
		"resources": []any{
			"catalog",
			gin.H{"name": "meta", "types": []string{stremioTypeAnime, stremioTypeMovie, stremioTypeSeries}, "idPrefixes": []string{rss.StremioIDPrefix}},
			gin.H{"name": "stream", "types": []string{stremioTypeAnime, stremioTypeMovie, stremioTypeSeries}, "idPrefixes": []string{rss.StremioIDPrefix}},
		},
		"idPrefixes": []string{rss.StremioIDPrefix},
	})
}

func (h *handlers) getCatalog(c *gin.Context) {
	typ := c.Param("type")
	if typ != stremioTypeAnime {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	cid, extras, ok := parseStremioCatalogPathParam(c.Param("catalogPath"))
	if !ok || (cid != catalogStremioID && cid != catalogStremioWeekID) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	snap := h.deps.Catalog.Snapshot()
	series := snap.Series
	if cid == catalogStremioWeekID {
		series = domain.FilterSeriesWithRecentReleases(&snap, 7)
		if series == nil {
			series = []domain.CatalogSeries{}
		}
	}
	if g := strings.TrimSpace(extras["genre"]); g != "" {
		series = domain.FilterSeriesByGenre(series, g)
	}
	c.JSON(http.StatusOK, gin.H{"metas": seriesToStremioMetas(series)})
}

func (h *handlers) getMeta(c *gin.Context) {
	typ := c.Param("type")
	id := stremioUnescapePathParam(strings.TrimSuffix(c.Param("meta_id"), ".json"))
	if !stremioMetaOrStreamTypeOK(typ) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	// Discover is under "anime"; some clients still request /meta/anime/:id for series rows.
	if domain.IsSeriesStremioID(id) {
		ser, ok := h.deps.Catalog.SeriesByID(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{})
			return
		}
		snap := h.deps.Catalog.Snapshot()
		groups := domain.GroupItemsByEpisode(snap.Items, id)
		keys := domain.OrderedEpisodeKeys(groups)
		en := h.deps.Catalog.AniListEnrichment(ser.ID)
		search := domain.AniListSearchQueryFromItems(snap.Items, ser.ID)
		if strings.TrimSpace(search) == "" {
			search = ser.Name
		}
		didLazyEnrich := false
		if strings.TrimSpace(search) != "" {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 14*time.Second)
			if h.deps.AniList != nil && domain.AniListNeedsRefetch(en) {
				det, err := h.deps.AniList.SearchAnimeMedia(ctx, search)
				if err == nil {
					add := anilist.ToDomainEnrichment(det)
					h.deps.Catalog.MergeAniListEnrichment(ser.ID, add)
					en = domain.MergeAniListEnrichment(en, add)
					didLazyEnrich = true
				}
			}
			if h.deps.Jikan != nil && domain.EnrichmentCouldUseJikan(en) {
				add, err := h.deps.Jikan.SearchAnimeEnrichment(ctx, search)
				if err == nil {
					h.deps.Catalog.MergeAniListEnrichment(ser.ID, add)
					en = domain.MergeAniListEnrichment(en, add)
					didLazyEnrich = true
				}
			}
			if h.deps.Kitsu != nil && domain.EnrichmentCouldUseJikan(en) {
				add, err := h.deps.Kitsu.SearchAnimeEnrichment(ctx, search)
				if err == nil {
					h.deps.Catalog.MergeAniListEnrichment(ser.ID, add)
					en = domain.MergeAniListEnrichment(en, add)
					didLazyEnrich = true
				}
			}
			if h.deps.TMDB != nil && strings.TrimSpace(en.StremioHeroBackgroundURL) == "" {
				cands, terr := services.TMDBBackdropCandidatesForEnrichment(ctx, h.deps.TMDB, en, search)
				if terr == nil {
					hero := strings.TrimSpace(domain.ResolveStremioHeroBackground(en, cands))
					if hero != "" {
						h.deps.Catalog.ReplaceStremioHeroBackground(ser.ID, hero)
						en.StremioHeroBackgroundURL = hero
						didLazyEnrich = true
					}
				}
			}
			if h.deps.Jikan != nil && en.MalID > 0 {
				eps, jerr := h.deps.Jikan.FetchEpisodeTitlesByMalID(ctx, en.MalID)
				if jerr == nil && len(eps) > 0 {
					addEp := domain.AniListSeriesEnrichment{EpisodeTitleByNum: eps}
					h.deps.Catalog.MergeAniListEnrichment(ser.ID, addEp)
					en = domain.MergeAniListEnrichment(en, addEp)
					didLazyEnrich = true
				}
			}
			en = h.deps.Catalog.AniListEnrichment(ser.ID)
			kitsuID := strings.TrimSpace(en.KitsuAnimeID)
			if kitsuID == "" && h.deps.Kitsu != nil && strings.TrimSpace(search) != "" {
				if id, kerr := h.deps.Kitsu.SearchAnimeID(ctx, search); kerr == nil && id != "" {
					addK := domain.AniListSeriesEnrichment{KitsuAnimeID: id}
					h.deps.Catalog.MergeAniListEnrichment(ser.ID, addK)
					en = domain.MergeAniListEnrichment(en, addK)
					kitsuID = id
					didLazyEnrich = true
				}
			}
			if h.deps.Kitsu != nil && kitsuID != "" && (len(en.EpisodeTitleByNum) == 0 || len(en.EpisodeThumbnailByNum) == 0) {
				t, th, kerr := h.deps.Kitsu.FetchEpisodeMaps(ctx, kitsuID)
				if kerr == nil && (len(t) > 0 || len(th) > 0) {
					addKE := domain.AniListSeriesEnrichment{KitsuAnimeID: kitsuID, EpisodeTitleByNum: t, EpisodeThumbnailByNum: th}
					h.deps.Catalog.MergeAniListEnrichment(ser.ID, addKE)
					en = domain.MergeAniListEnrichment(en, addKE)
					didLazyEnrich = true
				}
			}
			cancel()
		}
		synopsisUpdated := false
		if didLazyEnrich {
			enAfter := h.deps.Catalog.AniListEnrichment(ser.ID)
			newDesc := services.TranslateSynopsisToPT(h.deps.SynopsisTrans, h.deps.Log, enAfter.Description)
			if newDesc != enAfter.Description && strings.TrimSpace(newDesc) != "" {
				h.deps.Catalog.ReplaceAniListSynopsis(ser.ID, newDesc)
				synopsisUpdated = true
			}
		}
		if s2, ok := h.deps.Catalog.SeriesByID(id); ok {
			ser = s2
		}
		en = h.deps.Catalog.AniListEnrichment(ser.ID)
		// Sinopse vinha muitas vezes só em EN do sync; géneros já vão para PT por mapa. Traduz ao abrir a série
		// sinopse en→pt via gilang quando o corpo parecer inglês (idempotente se já estiver em PT).
		if h.deps.SynopsisTrans != nil && strings.TrimSpace(en.Description) != "" {
			newDesc := services.TranslateSynopsisToPT(h.deps.SynopsisTrans, h.deps.Log, en.Description)
			if newDesc != en.Description && strings.TrimSpace(newDesc) != "" {
				h.deps.Catalog.ReplaceAniListSynopsis(ser.ID, newDesc)
				en = h.deps.Catalog.AniListEnrichment(ser.ID)
				synopsisUpdated = true
			}
		}
		if didLazyEnrich || synopsisUpdated {
			pctx, pcancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := h.deps.Catalog.PersistActiveCatalog(pctx); err != nil && h.deps.Log != nil {
				h.deps.Log.Warn("persist catalog after Stremio meta enrichment", slog.Any("err", err))
			}
			pcancel()
		}
		videos := make([]gin.H, 0, len(keys))
		for _, k := range keys {
			group := groups[k]
			if len(group) == 0 {
				continue
			}
			vid := domain.EpisodeVideoStremioID(ser.ID, k.Season, k.Episode, k.Special)
			epNum := k.Episode
			if k.Special {
				epNum = 0
			}
			row := gin.H{
				"id":       vid,
				"title":    domain.EpisodeListTitleForGroup(k.Episode, k.Special, en.EpisodeTitleByNum, group),
				"released": stremioVideoReleasedISO(domain.LatestReleased(group)),
				"season":   k.Season,
				"episode":  epNum,
			}
			if !k.Special {
				if th := strings.TrimSpace(en.EpisodeThumbnailByNum[k.Episode]); th != "" {
					row["thumbnail"] = th
				}
			}
			videos = append(videos, row)
		}
		calSeason := domain.MaxSeasonAmongSeriesItems(snap.Items, id)
		appendAniListCalendarVideo(ser.ID, calSeason, en.NextAiringEpisode, en, groups, &videos)
		sortStremioMetaVideosByReleased(videos)
		// Must match catalog manifest type ("anime"). If meta.type is "series" while the catalog is
		// "anime", many Stremio clients skip synopsis, genres, and similar fields on the detail screen.
		meta := gin.H{
			"id":          ser.ID,
			"type":        stremioTypeAnime,
			"name":        ser.Name,
			"poster":      ser.Poster,
			"genres":      []string{"Anime"},
			"description": "Torrent releases with pt-BR subtitles (Erai).",
			"videos":      videos,
		}
		mergeAniListSeriesMeta(meta, en)
		c.JSON(http.StatusOK, gin.H{"meta": meta})
		return
	}
	it, ok := h.deps.Catalog.ItemByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	meta := gin.H{
		"id":   it.ID,
		"type": it.Type,
		"name": it.Name,
	}
	if it.Poster != "" {
		meta["poster"] = it.Poster
	}
	if it.Released != "" {
		meta["releaseInfo"] = it.Released
	}
	if it.SubtitlesTag != "" {
		meta["description"] = "Subtitles: " + it.SubtitlesTag
	}
	c.JSON(http.StatusOK, gin.H{"meta": meta})
}

func (h *handlers) getStream(c *gin.Context) {
	typ := c.Param("type")
	id := stremioUnescapePathParam(strings.TrimSuffix(c.Param("stream_id"), ".json"))
	if !stremioMetaOrStreamTypeOK(typ) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	if domain.IsEpisodeVideoStremioID(id) {
		snap := h.deps.Catalog.Snapshot()
		releases := domain.ItemsForEpisodeVideoID(snap.Items, id)
		if len(releases) == 0 {
			// Calendário pode listar episódio agendado (AniList) antes do RSS; ainda sem torrent.
			c.JSON(http.StatusOK, gin.H{"streams": []any{}})
			return
		}
		var epDisplay string
		if it0 := releases[0]; it0.SeriesID != "" {
			en := snap.AniListBySeries[it0.SeriesID]
			epDisplay = domain.EpisodeListTitleForGroup(it0.Episode, it0.IsSpecial, en.EpisodeTitleByNum, releases)
		}
		streams := make([]gin.H, 0, len(releases))
		for _, it := range releases {
			if s := streamFromCatalogItem(it, id, epDisplay); s != nil {
				streams = append(streams, s)
			}
		}
		if len(streams) == 0 {
			c.JSON(http.StatusOK, gin.H{"streams": []any{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"streams": streams})
		return
	}

	it, ok := h.deps.Catalog.ItemByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	var epDisplay string
	if it.SeriesID != "" && (it.Episode > 0 || it.IsSpecial) {
		snap := h.deps.Catalog.Snapshot()
		en := snap.AniListBySeries[it.SeriesID]
		epDisplay = domain.EpisodeListTitleForGroup(it.Episode, it.IsSpecial, en.EpisodeTitleByNum, []domain.CatalogItem{it})
	}
	one := streamFromCatalogItem(it, it.ID, epDisplay)
	if one == nil {
		c.JSON(http.StatusOK, gin.H{"streams": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"streams": []gin.H{one}})
}

func streamFromCatalogItem(it domain.CatalogItem, bingeGroup string, episodeDisplay string) gin.H {
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
		return gin.H{
			"name":     streamName,
			"title":    streamTitle,
			"infoHash": it.InfoHash,
			"fileIdx":  0,
			"behaviorHints": gin.H{
				"bingeGroup": bingeGroup,
			},
		}
	}
	if it.MagnetURL != "" {
		return gin.H{
			"name":  streamName,
			"title": streamTitle,
			"url":   it.MagnetURL,
		}
	}
	if it.TorrentURL != "" {
		return gin.H{
			"name":  streamName,
			"title": streamTitle,
			"url":   it.TorrentURL,
		}
	}
	return nil
}

func mergeAniListSeriesMeta(meta gin.H, en domain.AniListSeriesEnrichment) {
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
		meta["trailers"] = []gin.H{{"source": en.TrailerYouTubeID, "type": "Trailer"}}
	}
	if tp := strings.TrimSpace(en.TitlePreferred); tp != "" {
		meta["name"] = tp
	}
	if u := strings.TrimSpace(en.PosterURL); u != "" {
		meta["poster"] = u
	}
}
