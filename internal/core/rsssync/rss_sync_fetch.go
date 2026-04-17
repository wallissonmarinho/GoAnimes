package rsssync

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/btmeta"
	rssadapter "github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

type eraiSlugJob struct {
	slug, origin, token string
}

// collectMergedCatalogItems fetches main RSS sources, Erai per-anime feeds, merges with previous items, and backfills info_hash.
func (s *RSSSyncService) collectMergedCatalogItems(ctx context.Context, sources []domain.RSSSource, prevSnap domain.CatalogSnapshot, defaultEraiToken string) (merged []domain.CatalogItem, errs []string) {
	skipKnown := rssadapter.BuildFeedSyncSkip(prevSnap.Items)
	var rssBatch []domain.CatalogItem
	var eraiJobs []eraiSlugJob
	slugQueued := make(map[string]struct{})
	for _, src := range sources {
		body, status, hdr, gerr := s.getter.GetBytesGETRetryWithHeaders(ctx, src.URL, 3, 2*time.Second, "", "")
		if gerr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.Label, gerr))
			continue
		}
		if status != http.StatusOK || len(body) == 0 {
			errs = append(errs, fmt.Sprintf("%s: http status %d", src.Label, status))
			continue
		}
		s.ingestRSSMainFeedProbe(src.URL, body, hdr)
		items, slugs, perr := rssadapter.ParseFeedWithEraiSlugs(body, skipKnown)
		if perr != nil {
			errs = append(errs, fmt.Sprintf("%s: parse: %v", src.Label, perr))
			continue
		}
		rssBatch = append(rssBatch, items...)
		origin, token := rssadapter.EraiSourceOriginAndToken(src.URL)
		if origin == "" {
			continue
		}
		if token == "" {
			token = defaultEraiToken
		}
		if token == "" {
			continue
		}
		for _, slug := range slugs {
			if _, dup := slugQueued[slug]; dup {
				continue
			}
			slugQueued[slug] = struct{}{}
			eraiJobs = append(eraiJobs, eraiSlugJob{slug: slug, origin: origin, token: token})
		}
	}
	perAnimeCap := maxEraiPerAnimeFeedFetches()
	perAnimeOK := 0
	perAnimeDelay := eraiPerAnimeFetchDelay()
	perAnimeAttempts := eraiPerAnimeFetchMaxAttempts()
	perAnimeBackoff := eraiPerAnimeRetryBaseBackoff()
	for i, job := range eraiJobs {
		if perAnimeCap > 0 && perAnimeOK >= perAnimeCap {
			break
		}
		if i > 0 && perAnimeDelay > 0 {
			select {
			case <-ctx.Done():
				errs = append(errs, fmt.Sprintf("erai anime-list: cancelled during pacing (%v)", ctx.Err()))
				goto eraiPerAnimeDone
			case <-time.After(perAnimeDelay):
			}
		}
		u := rssadapter.BuildEraiPerAnimeFeedURL(job.origin, job.slug, job.token)
		if u == "" {
			continue
		}
		body, ferr := s.getter.GetBytesGETRetry(ctx, u, perAnimeAttempts, perAnimeBackoff)
		if ferr != nil {
			errs = append(errs, fmt.Sprintf("erai anime-list %s: %v", job.slug, ferr))
			continue
		}
		subItems, perr := rssadapter.ParseFeed(body)
		perAnimeOK++
		if perr != nil {
			errs = append(errs, fmt.Sprintf("erai anime-list %s: parse: %v", job.slug, perr))
			continue
		}
		rssBatch = append(rssBatch, subItems...)
	}
eraiPerAnimeDone:
	rssBatch, droppedBatch := domain.DropBatchCatalogItems(rssBatch)
	if droppedBatch > 0 {
		s.log.Info("catalog merge: dropped batch releases", slog.Int("dropped", droppedBatch))
	}
	s.log.Info("erai per-anime rss",
		slog.Int("slugs_queued", len(eraiJobs)),
		slog.Int("fetched", perAnimeOK),
		slog.Int("per_anime_cap", perAnimeCap),
		slog.Duration("per_anime_delay", perAnimeDelay),
		slog.Int("per_anime_max_attempts", perAnimeAttempts),
	)
	merged = domain.MergeCatalogItemsByID(prevSnap.Items, rssBatch)
	s.log.Info("catalog merge",
		slog.Int("from_feed_this_run", len(rssBatch)),
		slog.Int("total_episodes", len(merged)),
	)
	s.backfillTorrentInfoHashes(ctx, &merged, &errs)
	return merged, errs
}

func (s *RSSSyncService) backfillTorrentInfoHashes(ctx context.Context, merged *[]domain.CatalogItem, errs *[]string) {
	for i := range *merged {
		select {
		case <-ctx.Done():
			s.log.Warn("torrent info_hash backfill stopped early",
				slog.Any("err", ctx.Err()),
				slog.Int("processed_index", i),
				slog.Int("total_items", len(*merged)))
			*errs = append(*errs, fmt.Sprintf("torrent info_hash backfill: stopped early (%v)", ctx.Err()))
			return
		default:
		}
		it := &(*merged)[i]
		if it.InfoHash != "" || it.TorrentURL == "" {
			continue
		}
		body, err := s.getter.GetBytes(it.TorrentURL)
		if err != nil {
			s.log.Warn("fetch .torrent for info_hash", slog.String("url", it.TorrentURL), slog.Any("err", err))
			continue
		}
		h, err := btmeta.InfoHashHexFromTorrentBody(body)
		if err != nil {
			s.log.Warn("parse .torrent", slog.String("url", it.TorrentURL), slog.Any("err", err))
			continue
		}
		it.InfoHash = h
	}
}
