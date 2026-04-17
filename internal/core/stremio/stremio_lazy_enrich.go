package stremio

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/thetvdb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

// StremioLazyEnrichDeps holds optional API clients for lazy Stremio meta enrichment.
type StremioLazyEnrichDeps struct {
	TMDB          *tmdb.Client
	TheTVDB       *thetvdb.Client
	SynopsisTrans ports.SynopsisTranslator
}

// StremioLazyEnrichSeries fetches remote metadata and merges into the catalog (mutates via cat).
// Returns whether any remote data was merged and whether synopsis was replaced for persistence.
func StremioLazyEnrichSeries(ctx context.Context, log *slog.Logger, cat ports.CatalogAdmin, d StremioLazyEnrichDeps, ser domain.CatalogSeries, snap domain.CatalogSnapshot) (didLazyEnrich bool, synopsisUpdated bool) {
	if log == nil {
		log = slog.Default()
	}
	en := cat.SeriesEnrichment(ser.ID)
	search := domain.ExternalSearchQueryFromItems(snap.Items, ser.ID)
	if strings.TrimSpace(search) == "" {
		search = ser.Name
	}
	if strings.TrimSpace(search) == "" {
		return false, false
	}

	ctx, cancel := context.WithTimeout(ctx, 14*time.Second)
	defer cancel()

	if strings.TrimSpace(en.StremioHeroBackgroundURL) == "" && (d.TMDB != nil || d.TheTVDB != nil) {
		var combined []domain.BackgroundCandidate
		if d.TMDB != nil {
			if cands, terr := services.TMDBBackdropCandidatesForEnrichment(ctx, d.TMDB, en, search); terr == nil {
				combined = append(combined, cands...)
			}
		}
		if d.TheTVDB != nil {
			if cands, terr := services.TVDBBackdropCandidatesForEnrichment(ctx, d.TheTVDB, en); terr == nil {
				combined = append(combined, cands...)
			}
		}
		if hero := strings.TrimSpace(domain.ResolveStremioHeroBackground(en, combined)); hero != "" {
			cat.ReplaceStremioHeroBackground(ser.ID, hero)
			en.StremioHeroBackgroundURL = hero
			didLazyEnrich = true
		}
	}
	if didLazyEnrich {
		enAfter := cat.SeriesEnrichment(ser.ID)
		newDesc := services.TranslateSynopsisToPT(d.SynopsisTrans, log, enAfter.Description)
		if newDesc != enAfter.Description && strings.TrimSpace(newDesc) != "" {
			cat.ReplaceSeriesSynopsis(ser.ID, newDesc)
			synopsisUpdated = true
		}
	}
	if d.SynopsisTrans != nil {
		en = cat.SeriesEnrichment(ser.ID)
		if strings.TrimSpace(en.Description) != "" {
			newDesc := services.TranslateSynopsisToPT(d.SynopsisTrans, log, en.Description)
			if newDesc != en.Description && strings.TrimSpace(newDesc) != "" {
				cat.ReplaceSeriesSynopsis(ser.ID, newDesc)
				synopsisUpdated = true
			}
		}
	}
	return didLazyEnrich, synopsisUpdated
}
