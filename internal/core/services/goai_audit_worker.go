package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// GoaiAuditWorker runs one pass over catalog series: series audit when needed, then unaudited releases.
// On any GoAI HTTP error it stops immediately; the next run is on the scheduler interval only.
type GoaiAuditWorker struct {
	Repo   ports.GoAIAuditRepository
	Client ports.GoAIAuditHTTPClient
	Log    *slog.Logger
}

// Run executes a single audit pass. Returns early without error if there is nothing to do.
func (w *GoaiAuditWorker) Run(ctx context.Context) {
	if w.Log == nil {
		w.Log = slog.Default()
	}
	if w.Repo == nil || w.Client == nil {
		w.Log.Warn("goai audit: repo or client nil, skip")
		return
	}
	ids, err := w.Repo.ListSeriesIDsWithCatalogItems(ctx)
	if err != nil {
		w.Log.Error("goai audit: list series ids", slog.Any("err", err))
		return
	}
	for _, sid := range ids {
		if ctx.Err() != nil {
			return
		}
		rec, err := w.Repo.GetSeriesAudit(ctx, sid)
		if err != nil {
			w.Log.Error("goai audit: get series audit", slog.String("series_id", sid), slog.Any("err", err))
			return
		}
		needSeries := rec == nil || rec.NeedsReaudit || rec.PromptVersion < domain.GoaiAuditPromptVersion
		if needSeries {
			sampleCtx, err := w.Repo.SampleItemContextForSeries(ctx, sid)
			if err != nil {
				w.Log.Error("goai audit: sample series context", slog.String("series_id", sid), slog.Any("err", err))
				return
			}
			if sampleCtx == nil || sampleCtx.Title == "" {
				w.Log.Debug("goai audit: skip series (no item title)", slog.String("series_id", sid))
				continue
			}
			sname, err := w.Repo.GetCatalogSeriesName(ctx, sid)
			if err != nil {
				w.Log.Error("goai audit: series name", slog.String("series_id", sid), slog.Any("err", err))
				return
			}
			tvdb, err := w.Repo.GetEnrichmentTVDBSeriesID(ctx, sid)
			if err != nil {
				w.Log.Error("goai audit: enrichment tvdb", slog.String("series_id", sid), slog.Any("err", err))
				return
			}
			req := domain.GoaiSeriesAuditRequest{
				TorrentTitle:         sampleCtx.Title,
				TorrentLink:          sampleCtx.TorrentURL,
				FeedPublishedAt:      sampleCtx.Released,
				ParsedSeasonHint:     sampleCtx.Season,
				ParsedEpisodeHint:    sampleCtx.Episode,
				ParsedIsSpecialHint:  sampleCtx.IsSpecial,
				SeriesName:           sname,
				SeriesID:             sid,
				ExistingTVDBSeriesID: tvdb,
			}
			resp, err := w.Client.AuditSeries(ctx, req)
			if err != nil {
				w.Log.Error("goai audit: AuditSeries stopped run", slog.String("series_id", sid), slog.Any("err", err))
				return
			}
			raw, err := json.Marshal(resp)
			if err != nil {
				w.Log.Error("goai audit: marshal series response", slog.Any("err", err))
				return
			}
			now := time.Now().UTC()
			if err := w.Repo.UpsertSeriesAudit(ctx, sid, now, domain.GoaiAuditPromptVersion, string(raw), false); err != nil {
				w.Log.Error("goai audit: upsert series audit", slog.String("series_id", sid), slog.Any("err", err))
				return
			}
			if resp.TheTVDBSeriesID > 0 {
				if err := w.Repo.UpdateSeriesEnrichmentTVDB(ctx, sid, resp.TheTVDBSeriesID); err != nil {
					w.Log.Error("goai audit: update tvdb enrichment", slog.String("series_id", sid), slog.Any("err", err))
					return
				}
			}
		}

		keys, err := w.Repo.ListUnauditedReleaseKeysForSeries(ctx, sid)
		if err != nil {
			w.Log.Error("goai audit: list unaudited releases", slog.String("series_id", sid), slog.Any("err", err))
			return
		}
		for _, k := range keys {
			if ctx.Err() != nil {
				return
			}
			itemCtx, err := w.Repo.SampleItemContextForRelease(ctx, k)
			if err != nil {
				w.Log.Error("goai audit: sample release context", slog.Any("key", k), slog.Any("err", err))
				return
			}
			itemTitle := ""
			if itemCtx != nil {
				itemTitle = itemCtx.Title
			}
			if itemTitle == "" {
				itemTitle = sampleFallbackTitle(ctx, w.Repo, sid)
			}
			sname, err := w.Repo.GetCatalogSeriesName(ctx, sid)
			if err != nil {
				w.Log.Error("goai audit: series name for release", slog.String("series_id", sid), slog.Any("err", err))
				return
			}
			req := domain.GoaiReleaseAuditRequest{
				TorrentTitle:   itemTitle,
				TorrentLink:    releaseField(itemCtx, func(x *domain.GoaiAuditItemContext) string { return x.TorrentURL }),
				FeedPublishedAt: releaseField(itemCtx, func(x *domain.GoaiAuditItemContext) string { return x.Released }),
				SeriesName:     sname,
				SeriesID:       sid,
				CurrentSeason:  k.Season,
				CurrentEpisode: k.Episode,
				IsSpecial:      k.IsSpecial,
			}
			resp, err := w.Client.AuditRelease(ctx, req)
			if err != nil {
				w.Log.Error("goai audit: AuditRelease stopped run", slog.Any("key", k), slog.Any("err", err))
				return
			}
			rb, err := json.Marshal(resp)
			if err != nil {
				w.Log.Error("goai audit: marshal release response", slog.Any("err", err))
				return
			}
			now := time.Now().UTC()
			if err := w.Repo.UpsertReleaseAudit(ctx, sid, resp.Season, resp.Episode, resp.IsSpecial, now, domain.GoaiAuditPromptVersion, string(rb), itemTitle); err != nil {
				w.Log.Error("goai audit: upsert release audit", slog.Any("key", k), slog.Any("err", err))
				return
			}
		}
	}
	w.Log.Info("goai audit: pass completed", slog.Int("series_count", len(ids)))
}

func sampleFallbackTitle(ctx context.Context, repo ports.GoAIAuditRepository, seriesID string) string {
	t, _ := repo.SampleItemTitleForSeries(ctx, seriesID)
	return t
}

func releaseField[T any](ctx *domain.GoaiAuditItemContext, pick func(*domain.GoaiAuditItemContext) T) T {
	var zero T
	if ctx == nil {
		return zero
	}
	return pick(ctx)
}
