package stremio

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/jikan"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/kitsu"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

// StremioLazyEnrichDeps holds optional API clients for lazy Stremio meta enrichment.
type StremioLazyEnrichDeps struct {
	AniList       *anilist.Client
	Jikan         *jikan.Client
	Kitsu         *kitsu.Client
	TMDB          *tmdb.Client
	SynopsisTrans ports.SynopsisTranslator
}

// StremioLazyEnrichSeries fetches remote metadata and merges into the catalog (mutates via cat).
// Returns whether any remote data was merged and whether synopsis was replaced for persistence.
func StremioLazyEnrichSeries(ctx context.Context, log *slog.Logger, cat ports.CatalogAdmin, d StremioLazyEnrichDeps, ser domain.CatalogSeries, snap domain.CatalogSnapshot) (didLazyEnrich bool, synopsisUpdated bool) {
	if log == nil {
		log = slog.Default()
	}
	en := cat.AniListEnrichment(ser.ID)
	search := domain.AniListSearchQueryFromItems(snap.Items, ser.ID)
	if strings.TrimSpace(search) == "" {
		search = ser.Name
	}
	if strings.TrimSpace(search) == "" {
		return false, false
	}

	ctx, cancel := context.WithTimeout(ctx, 14*time.Second)
	defer cancel()

	if d.AniList != nil && domain.AniListNeedsRefetch(en) {
		if det, err := d.AniList.SearchAnimeMedia(ctx, search); err == nil {
			add := anilist.ToDomainEnrichment(det)
			cat.MergeAniListEnrichment(ser.ID, add)
			en = domain.MergeAniListEnrichment(en, add)
			didLazyEnrich = true
		}
	}
	if d.Jikan != nil && domain.EnrichmentCouldUseJikan(en) {
		if add, err := d.Jikan.SearchAnimeEnrichment(ctx, search); err == nil {
			cat.MergeAniListEnrichment(ser.ID, add)
			en = domain.MergeAniListEnrichment(en, add)
			didLazyEnrich = true
		}
	}
	if d.Kitsu != nil && domain.EnrichmentCouldUseJikan(en) {
		if add, err := d.Kitsu.SearchAnimeEnrichment(ctx, search); err == nil {
			cat.MergeAniListEnrichment(ser.ID, add)
			en = domain.MergeAniListEnrichment(en, add)
			didLazyEnrich = true
		}
	}
	if d.TMDB != nil && strings.TrimSpace(en.StremioHeroBackgroundURL) == "" {
		if cands, terr := services.TMDBBackdropCandidatesForEnrichment(ctx, d.TMDB, en, search); terr == nil {
			if hero := strings.TrimSpace(domain.ResolveStremioHeroBackground(en, cands)); hero != "" {
				cat.ReplaceStremioHeroBackground(ser.ID, hero)
				en.StremioHeroBackgroundURL = hero
				didLazyEnrich = true
			}
		}
	}
	if d.Jikan != nil && en.MalID > 0 {
		if eps, jerr := d.Jikan.FetchEpisodeTitlesByMalID(ctx, en.MalID); jerr == nil && len(eps) > 0 {
			addEp := domain.AniListSeriesEnrichment{EpisodeTitleByNum: eps}
			cat.MergeAniListEnrichment(ser.ID, addEp)
			en = domain.MergeAniListEnrichment(en, addEp)
			didLazyEnrich = true
		}
	}
	en = cat.AniListEnrichment(ser.ID)
	kitsuID := strings.TrimSpace(en.KitsuAnimeID)
	if kitsuID == "" && d.Kitsu != nil && strings.TrimSpace(search) != "" {
		if id, kerr := d.Kitsu.SearchAnimeID(ctx, search); kerr == nil && id != "" {
			addK := domain.AniListSeriesEnrichment{KitsuAnimeID: id}
			cat.MergeAniListEnrichment(ser.ID, addK)
			en = domain.MergeAniListEnrichment(en, addK)
			kitsuID = id
			didLazyEnrich = true
		}
	}
	if d.Kitsu != nil && kitsuID != "" && (!domain.EnrichmentHasAnyEpisodeTitle(en) || len(en.EpisodeThumbnailByNum) == 0) {
		if t, th, kerr := d.Kitsu.FetchEpisodeMaps(ctx, kitsuID); kerr == nil && (len(t) > 0 || len(th) > 0) {
			addKE := domain.AniListSeriesEnrichment{KitsuAnimeID: kitsuID, EpisodeTitleByNum: t, EpisodeThumbnailByNum: th}
			cat.MergeAniListEnrichment(ser.ID, addKE)
			en = domain.MergeAniListEnrichment(en, addKE)
			didLazyEnrich = true
		}
	}

	if didLazyEnrich {
		enAfter := cat.AniListEnrichment(ser.ID)
		newDesc := services.TranslateSynopsisToPT(d.SynopsisTrans, log, enAfter.Description)
		if newDesc != enAfter.Description && strings.TrimSpace(newDesc) != "" {
			cat.ReplaceAniListSynopsis(ser.ID, newDesc)
			synopsisUpdated = true
		}
	}
	if d.SynopsisTrans != nil {
		en = cat.AniListEnrichment(ser.ID)
		if strings.TrimSpace(en.Description) != "" {
			newDesc := services.TranslateSynopsisToPT(d.SynopsisTrans, log, en.Description)
			if newDesc != en.Description && strings.TrimSpace(newDesc) != "" {
				cat.ReplaceAniListSynopsis(ser.ID, newDesc)
				synopsisUpdated = true
			}
		}
	}
	return didLazyEnrich, synopsisUpdated
}
