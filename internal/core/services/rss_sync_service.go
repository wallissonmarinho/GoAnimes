package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/btmeta"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/jikan"
	rssadapter "github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// maxPersistedSyncErrors caps lines stored in DB (RSS + enrichment); further issues stay in logs only.
const maxPersistedSyncErrors = 250

func appendSyncNote(notes *[]string, line string) {
	if notes == nil || len(*notes) >= maxPersistedSyncErrors {
		return
	}
	*notes = append(*notes, line)
}

func capPersistedSyncLines(lines []string) []string {
	if len(lines) <= maxPersistedSyncErrors {
		return lines
	}
	out := append([]string(nil), lines[:maxPersistedSyncErrors]...)
	return append(out, fmt.Sprintf("… and %d more (see server logs)", len(lines)-maxPersistedSyncErrors))
}

// RSSSyncRuntimeOptions configures outbound fetch.
type RSSSyncRuntimeOptions struct {
	HTTPTimeout     time.Duration
	MaxBodyBytes    int64
	UserAgent       string
	AniList         *anilist.Client
	AniListMinDelay time.Duration // sleep between AniList calls (rate limit); 0 → default 750ms
	Jikan           *jikan.Client
	JikanMinDelay   time.Duration // sleep after each Jikan series (2 HTTP calls); 0 → default 400ms
}

// RSSSyncService fetches RSS sources, filters pt-BR (Erai [br]), updates memory + DB snapshot.
type RSSSyncService struct {
	repo         ports.CatalogRepository
	mem          *state.CatalogStore
	getter       *httpclient.Getter
	anilist      *anilist.Client
	anilistDelay time.Duration
	jikan        *jikan.Client
	jikanDelay   time.Duration
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
	jdly := o.JikanMinDelay
	if jdly <= 0 {
		jdly = 400 * time.Millisecond
	}
	return &RSSSyncService{
		repo:         repo,
		mem:          mem,
		getter:       httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, o.MaxBodyBytes),
		anilist:      o.AniList,
		anilistDelay: dly,
		jikan:        o.Jikan,
		jikanDelay:   jdly,
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

func (s *RSSSyncService) enrichAniListSeries(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
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
			appendSyncNote(syncNotes, "anilist: stopped early (context cancelled)")
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
			appendSyncNote(syncNotes, fmt.Sprintf("anilist %q: %v", ser.Name, err))
			continue
		}
		cache[ser.ID] = anilist.ToDomainEnrichment(det)
		newN++
		if s.anilistDelay > 0 {
			time.Sleep(s.anilistDelay)
		}
	}
	s.log.Info("anilist: finished", slog.Int("new_or_refreshed", newN), slog.Int("lookup_failures", fails))
}

func (s *RSSSyncService) enrichJikanGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.jikan == nil {
		if s.jikan == nil {
			s.log.Info("jikan: skipped (set GOANIMES_JIKAN_DISABLED=true to disable; no client)")
		}
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("jikan: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "jikan: stopped early (context cancelled)")
			return
		default:
		}
		if strings.TrimSpace(ser.Name) == "" {
			continue
		}
		cur := cache[ser.ID]
		if !domain.EnrichmentCouldUseJikan(cur) {
			continue
		}
		add, err := s.jikan.SearchAnimeEnrichment(ctx, ser.Name)
		if err != nil {
			s.log.Debug("jikan lookup skipped or failed", slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("jikan %q: %v", ser.Name, err))
			continue
		}
		cache[ser.ID] = domain.MergeAniListEnrichment(cur, add)
		n++
		if s.jikanDelay > 0 {
			time.Sleep(s.jikanDelay)
		}
	}
	if n > 0 {
		s.log.Info("jikan: filled gaps", slog.Int("series", n))
	}
}

// Run fetches all RSS sources and rebuilds the catalog.
func (s *RSSSyncService) Run(ctx context.Context) domain.SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	started := time.Now().UTC()
	prevSnap, _ := s.repo.LoadCatalogSnapshot(ctx)
	anilistCache := cloneAniListCache(prevSnap.AniListBySeries)
	live := s.mem.Snapshot()
	for k, v := range live.AniListBySeries {
		anilistCache[k] = domain.MergeAniListEnrichment(anilistCache[k], v)
	}

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
		snap.LastSyncErrors = nil
		_ = s.mem.SetAndPersist(ctx, s.repo, snap)
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
	var enrichNotes []string
	s.enrichAniListSeries(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichJikanGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	pruneAniListCache(anilistCache, snap.Series)
	snap.AniListBySeries = anilistCache
	domain.ApplyAniListEnrichmentToSeries(&snap)
	snap.Message = fmt.Sprintf("synced %d episodes in %d series from %d feed(s)", len(merged), len(snap.Series), len(sources))
	snap.LastSyncErrors = capPersistedSyncLines(append(append([]string{}, errs...), enrichNotes...))
	if saveErr := s.mem.SetAndPersist(ctx, s.repo, snap); saveErr != nil {
		errs = append(errs, "save snapshot: "+saveErr.Error())
		s.log.Error("save snapshot", slog.Any("err", saveErr))
		snap.LastSyncErrors = capPersistedSyncLines(append(snap.LastSyncErrors, errs[len(errs)-1]))
		s.mem.Set(snap)
	}
	return domain.SyncResult{Message: snap.Message, Errors: errs}
}
