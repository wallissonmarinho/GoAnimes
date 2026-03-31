package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/btmeta"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	rssadapter "github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// RSSSyncRuntimeOptions configures outbound fetch.
type RSSSyncRuntimeOptions struct {
	HTTPTimeout     time.Duration
	MaxBodyBytes    int64
	UserAgent       string
	AniList         *anilist.Client
	AniListMinDelay time.Duration // sleep between AniList calls (rate limit); 0 → default 750ms
}

// RSSSyncService fetches RSS sources, filters pt-BR (Erai [br]), updates memory + DB snapshot.
type RSSSyncService struct {
	repo         ports.CatalogRepository
	mem          *state.CatalogStore
	getter       *httpclient.Getter
	anilist      *anilist.Client
	anilistDelay time.Duration
	log          *slog.Logger
	mu           sync.Mutex
}

func NewRSSSyncService(repo ports.CatalogRepository, mem *state.CatalogStore, o RSSSyncRuntimeOptions, log *slog.Logger) *RSSSyncService {
	if log == nil {
		log = slog.Default()
	}
	if o.HTTPTimeout <= 0 {
		o.HTTPTimeout = 45 * time.Second
	}
	if o.MaxBodyBytes <= 0 {
		o.MaxBodyBytes = 50 << 20
	}
	if o.UserAgent == "" {
		o.UserAgent = "GoAnimes/1.0"
	}
	dly := o.AniListMinDelay
	if dly <= 0 {
		dly = 750 * time.Millisecond
	}
	return &RSSSyncService{
		repo:         repo,
		mem:          mem,
		getter:       httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, o.MaxBodyBytes),
		anilist:      o.AniList,
		anilistDelay: dly,
		log:          log,
	}
}

func cloneAniListCache(m map[string]domain.AniListSeriesEnrichment) map[string]domain.AniListSeriesEnrichment {
	out := make(map[string]domain.AniListSeriesEnrichment)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func pruneAniListCache(m map[string]domain.AniListSeriesEnrichment, series []domain.CatalogSeries) {
	want := make(map[string]struct{}, len(series))
	for _, s := range series {
		want[s.ID] = struct{}{}
	}
	for id := range m {
		if _, ok := want[id]; !ok {
			delete(m, id)
		}
	}
}

func (s *RSSSyncService) enrichAniListSeries(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment) {
	if len(series) == 0 {
		return
	}
	if s.anilist == nil {
		s.log.Info("anilist: skipped (set GOANIMES_ANILIST_DISABLED=true to disable; no client)")
		return
	}
	missing := 0
	for _, ser := range series {
		if domain.AniListNeedsRefetch(cache[ser.ID]) {
			missing++
		}
	}
	if missing == 0 {
		s.log.Info("anilist: all series have full cached metadata", slog.Int("series", len(series)))
		return
	}
	s.log.Info("anilist: fetching metadata (posters, synopsis, genres, …)",
		slog.Int("to_fetch", missing),
		slog.Int("series_total", len(series)),
		slog.Duration("min_delay_between_requests", s.anilistDelay))

	newN, fails := 0, 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("anilist: stopped early (context done)", slog.Int("new_rows", newN), slog.Int("failures", fails))
			return
		default:
		}
		if !domain.AniListNeedsRefetch(cache[ser.ID]) {
			continue
		}
		det, err := s.anilist.SearchAnimeMedia(ctx, ser.Name)
		if err != nil {
			fails++
			s.log.Warn("anilist lookup failed", slog.String("series", ser.Name), slog.Any("err", err))
			continue
		}
		epTitles := det.EpisodeTitleByNum
		if epTitles == nil {
			epTitles = map[int]string{}
		}
		cache[ser.ID] = domain.AniListSeriesEnrichment{
			PosterURL:         det.PosterURL,
			BackgroundURL:     det.BackgroundURL,
			Description:       det.Description,
			Genres:            det.Genres,
			StartYear:         det.StartYear,
			EpisodeLengthMin:  det.EpisodeLengthMin,
			TrailerYouTubeID:  det.TrailerYouTubeID,
			TitlePreferred:    det.Title,
			EpisodeTitleByNum: epTitles,
		}
		newN++
		if s.anilistDelay > 0 {
			time.Sleep(s.anilistDelay)
		}
	}
	s.log.Info("anilist: finished", slog.Int("new_or_refreshed", newN), slog.Int("lookup_failures", fails))
}

// Run fetches all RSS sources and rebuilds the catalog.
func (s *RSSSyncService) Run(ctx context.Context) domain.SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	started := time.Now().UTC()
	prevSnap, _ := s.repo.LoadCatalogSnapshot(ctx)
	anilistCache := cloneAniListCache(prevSnap.AniListBySeries)

	sources, err := s.repo.ListRSSSources(ctx)
	if err != nil {
		return domain.SyncResult{Message: "list sources failed", Errors: []string{err.Error()}}
	}
	if len(sources) == 0 {
		snap := domain.CatalogSnapshot{
			OK:              true,
			Message:         "no rss sources configured",
			ItemCount:       0,
			StartedAt:       started,
			FinishedAt:      time.Now().UTC(),
			Items:           nil,
			AniListBySeries: anilistCache,
		}
		domain.EnsureSnapshotGrouped(&snap)
		domain.ApplyAniListEnrichmentToSeries(&snap)
		s.mem.Set(snap)
		_ = s.repo.SaveCatalogSnapshot(ctx, snap)
		return domain.SyncResult{Message: snap.Message}
	}

	byID := make(map[string]domain.CatalogItem)
	var errs []string
	for _, src := range sources {
		body, gerr := s.getter.GetBytes(src.URL)
		if gerr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.Label, gerr))
			continue
		}
		items, perr := rssadapter.ParseFeed(body)
		if perr != nil {
			errs = append(errs, fmt.Sprintf("%s: parse: %v", src.Label, perr))
			continue
		}
		for _, it := range items {
			byID[it.ID] = it
		}
	}
	merged := make([]domain.CatalogItem, 0, len(byID))
	for _, it := range byID {
		merged = append(merged, it)
	}

	for i := range merged {
		it := &merged[i]
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

	snap := domain.CatalogSnapshot{
		OK:         true,
		ItemCount:  len(merged),
		StartedAt:  started,
		FinishedAt: time.Now().UTC(),
		Items:      merged,
	}
	domain.EnsureSnapshotGrouped(&snap)
	s.enrichAniListSeries(ctx, snap.Series, anilistCache)
	pruneAniListCache(anilistCache, snap.Series)
	snap.AniListBySeries = anilistCache
	domain.ApplyAniListEnrichmentToSeries(&snap)
	snap.Message = fmt.Sprintf("synced %d episodes in %d series from %d feed(s)", len(merged), len(snap.Series), len(sources))
	s.mem.Set(snap)
	if saveErr := s.repo.SaveCatalogSnapshot(ctx, snap); saveErr != nil {
		errs = append(errs, "save snapshot: "+saveErr.Error())
		s.log.Error("save snapshot", slog.Any("err", saveErr))
	}
	return domain.SyncResult{Message: snap.Message, Errors: errs}
}
