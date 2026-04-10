package ginapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/stremio"
)

const (
	catalogStremioID     = "goanimes"
	catalogStremioWeekID = "goanimes-week"
	stremioTypeAnime     = "anime"
	stremioTypeMovie     = "movie"
	stremioTypeSeries    = "series"
	// stremioManifestVersion: PATCH = fixes, tuning, deps, docs; MINOR = nova funcionalidade visível (API, sync, catálogo Stremio); MAJOR = contrato que parte instalações.
	stremioManifestVersion = "1.8.1"
)

func stremioMetaOrStreamTypeOK(t string) bool {
	switch t {
	case stremioTypeAnime, stremioTypeMovie, stremioTypeSeries:
		return true
	default:
		return false
	}
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

func (h *handlers) getManifest(c *gin.Context) {
	snap := h.deps.Catalog.Snapshot()
	genreOpts := domain.UniqueGenreLabelsFromCatalogSeries(snap.Series)
	genreExtra := []gin.H{{
		"name":       "genre",
		"isRequired": false,
		"options":    genreOpts,
	}}
	genres := genreOpts
	manifestDesc := "RSS anime torrents with pt-BR (Erai [br]) filter."
	if h.deps.TheTVDB != nil {
		manifestDesc += " Parte dos metadados (episódios, imagens) pode vir do TheTVDB — https://www.thetvdb.com/ (atribuição exigida pelo serviço)."
	}
	c.JSON(http.StatusOK, gin.H{
		"id":          "org.goanimes",
		"version":     stremioManifestVersion,
		"name":        "GoAnimes",
		"description": manifestDesc,
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
	metas := stremio.SeriesToStremioMetaMaps(series)
	c.JSON(http.StatusOK, gin.H{"metas": metas})
}

func (h *handlers) getMeta(c *gin.Context) {
	typ := c.Param("type")
	id := stremioUnescapePathParam(strings.TrimSuffix(c.Param("meta_id"), ".json"))
	if !stremioMetaOrStreamTypeOK(typ) {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	if domain.IsSeriesStremioID(id) {
		ser, ok := h.deps.Catalog.SeriesByID(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{})
			return
		}
		snap := h.deps.Catalog.Snapshot()
		groups := domain.GroupItemsByEpisode(snap.Items, id)
		keys := domain.OrderedEpisodeKeys(groups)
		enrichDeps := stremio.StremioLazyEnrichDeps{
			AniList:       h.deps.AniList,
			Jikan:         h.deps.Jikan,
			Kitsu:         h.deps.Kitsu,
			TMDB:          h.deps.TMDB,
			TheTVDB:       h.deps.TheTVDB,
			SynopsisTrans: h.deps.SynopsisTrans,
		}
		didLazyEnrich, synopsisUpdated := stremio.StremioLazyEnrichSeries(
			c.Request.Context(), h.deps.Log, h.deps.Catalog, enrichDeps, ser, snap,
		)
		if s2, ok := h.deps.Catalog.SeriesByID(id); ok {
			ser = s2
		}
		en := h.deps.Catalog.AniListEnrichment(ser.ID)
		if didLazyEnrich || synopsisUpdated {
			pctx, pcancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := h.deps.Catalog.PersistActiveCatalog(pctx); err != nil && h.deps.Log != nil {
				h.deps.Log.Warn("persist catalog after Stremio meta enrichment", slog.Any("err", err))
			}
			pcancel()
		}
		videos := make([]map[string]any, 0, len(keys))
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
			row := map[string]any{
				"id":       vid,
				"title":    domain.EpisodeListTitleForGroup(k.Episode, k.Special, en.EpisodeTitleByNum, group),
				"released": stremio.StremioVideoReleasedISO(domain.LatestReleased(group)),
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
		stremio.AppendAniListCalendarVideo(ser.ID, calSeason, en.NextAiringEpisode, en, groups, &videos)
		stremio.SortStremioMetaVideosByReleased(videos)
		meta := map[string]any{
			"id":          ser.ID,
			"type":        stremioTypeAnime,
			"name":        ser.Name,
			"poster":      ser.Poster,
			"genres":      []string{"Anime"},
			"description": "Torrent releases with pt-BR subtitles (Erai).",
			"videos":      videos,
		}
		stremio.MergeAniListIntoStremioSeriesMeta(meta, en)
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
			c.JSON(http.StatusOK, gin.H{"streams": []any{}})
			return
		}
		var epDisplay string
		if it0 := releases[0]; it0.SeriesID != "" {
			en := snap.AniListBySeries[it0.SeriesID]
			epDisplay = domain.EpisodeListTitleForGroup(it0.Episode, it0.IsSpecial, en.EpisodeTitleByNum, releases)
		}
		streams := make([]map[string]any, 0, len(releases))
		for _, it := range releases {
			if s := stremio.StreamFromCatalogItem(it, id, epDisplay); s != nil {
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
	one := stremio.StreamFromCatalogItem(it, it.ID, epDisplay)
	if one == nil {
		c.JSON(http.StatusOK, gin.H{"streams": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"streams": []gin.H{gin.H(one)}})
}
