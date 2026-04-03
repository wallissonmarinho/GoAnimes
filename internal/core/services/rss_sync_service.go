package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// maxEraiPerAnimeFeedFetches limits HTTP fetches to anime-list/{slug}/feed per sync (0 = unlimited).
func maxEraiPerAnimeFeedFetches() int {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_MAX_PER_ANIME_FEEDS"))
	if v == "" {
		return 200
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 200
	}
	return n
}

// RSSSyncRuntimeOptions configures outbound fetch.
type RSSSyncRuntimeOptions struct {
	HTTPTimeout     time.Duration
	MaxBodyBytes    int64
	UserAgent       string
	AniList         *anilist.Client
	AniListMinDelay time.Duration // extra sleep after each AniList success (client already paces requests); default 0
	Jikan           *jikan.Client
	JikanMinDelay   time.Duration // sleep after each Jikan enrichment; 0 → default 900ms (client also paces each HTTP call)
	SynopsisTrans   ports.SynopsisTranslator // optional: synopsis translation (gilang googletranslate when enabled)
}

// RSSSyncService fetches RSS sources, filters pt-BR (Erai [br]), updates memory + DB snapshot.
type RSSSyncService struct {
	repo         ports.CatalogRepository
	mem          *state.CatalogStore
	getter       *httpclient.Getter
	anilist      *anilist.Client
	anilistDelay time.Duration
	jikan         *jikan.Client
	jikanDelay    time.Duration
	synopsisTrans ports.SynopsisTranslator
	log           *slog.Logger
	mu            sync.Mutex
	syncRunning   atomic.Bool
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
	if dly < 0 {
		dly = 0
	}
	jdly := o.JikanMinDelay
	if jdly <= 0 {
		jdly = 900 * time.Millisecond
	}
	return &RSSSyncService{
		repo:          repo,
		mem:           mem,
		getter:        httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, o.MaxBodyBytes),
		anilist:       o.AniList,
		anilistDelay:  dly,
		jikan:         o.Jikan,
		jikanDelay:    jdly,
		synopsisTrans: o.SynopsisTrans,
		log:           log,
	}
}

// SyncRunning reports whether Run is in progress.
func (s *RSSSyncService) SyncRunning() bool {
	if s == nil {
		return false
	}
	return s.syncRunning.Load()
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
			qlog := domain.NormalizeExternalAnimeSearchQuery(ser.Name)
			s.log.Warn("anilist lookup failed", slog.String("series", ser.Name), slog.String("search_query", qlog), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("anilist %q: %v", qlog, err))
			continue
		}
		en := anilist.ToDomainEnrichment(det)
		en.Description = TranslateSynopsisToPT(s.synopsisTrans, s.log, en.Description)
		cache[ser.ID] = en
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
		merged := domain.MergeAniListEnrichment(cur, add)
		merged.Description = TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
		cache[ser.ID] = merged
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
	s.syncRunning.Store(true)
	defer s.syncRunning.Store(false)

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
	// Erai per-anime URLs need ?token=; use the token from any registered Erai feed URL (same account).
	var defaultEraiToken string
	for _, src := range sources {
		if _, tok := rssadapter.EraiSourceOriginAndToken(src.URL); tok != "" {
			defaultEraiToken = tok
			break
		}
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

	var rssBatch []domain.CatalogItem
	var errs []string
	type eraiSlugJob struct {
		slug, origin, token string
	}
	var eraiJobs []eraiSlugJob
	slugQueued := make(map[string]struct{})
	for _, src := range sources {
		body, gerr := s.getter.GetBytes(src.URL)
		if gerr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.Label, gerr))
			continue
		}
		items, slugs, perr := rssadapter.ParseFeedWithEraiSlugs(body)
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
	for _, job := range eraiJobs {
		if perAnimeCap > 0 && perAnimeOK >= perAnimeCap {
			break
		}
		u := rssadapter.BuildEraiPerAnimeFeedURL(job.origin, job.slug, job.token)
		if u == "" {
			continue
		}
		body, ferr := s.getter.GetBytes(u)
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
	s.log.Info("erai per-anime rss",
		slog.Int("slugs_queued", len(eraiJobs)),
		slog.Int("fetched", perAnimeOK),
		slog.Int("per_anime_cap", perAnimeCap),
	)
	merged := domain.MergeCatalogItemsByID(prevSnap.Items, rssBatch)
	s.log.Info("catalog merge",
		slog.Int("from_feed_this_run", len(rssBatch)),
		slog.Int("total_episodes", len(merged)),
	)

	for i := range merged {
		select {
		case <-ctx.Done():
			s.log.Warn("torrent info_hash backfill stopped early",
				slog.Any("err", ctx.Err()),
				slog.Int("processed_index", i),
				slog.Int("total_items", len(merged)))
			errs = append(errs, fmt.Sprintf("torrent info_hash backfill: stopped early (%v)", ctx.Err()))
			goto doneTorrentBackfill
		default:
		}
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
doneTorrentBackfill:

	snap := domain.CatalogSnapshot{
		OK:         true,
		ItemCount:  len(merged),
		StartedAt:  started,
		FinishedAt: time.Now().UTC(),
		Items:      merged,
	}
	domain.EnsureSnapshotGrouped(&snap)
	domain.SortCatalogItemsInPlace(snap.Items)
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
