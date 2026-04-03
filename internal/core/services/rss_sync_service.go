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
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/kitsu"
	rssadapter 	"github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
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

// eraiPerAnimeFetchDelay is the pause between successive per-anime Erai feed GETs (reduces 429).
func eraiPerAnimeFetchDelay() time.Duration {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_PER_ANIME_DELAY"))
	if v == "" {
		return 400 * time.Millisecond
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 400 * time.Millisecond
	}
	return d
}

// eraiPerAnimeFetchMaxAttempts is GET retries per slug on 429/503 (default 5 = 1 try + up to 4 backoff waits).
func eraiPerAnimeFetchMaxAttempts() int {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_PER_ANIME_MAX_ATTEMPTS"))
	if v == "" {
		return 5
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 5
	}
	if n > 20 {
		return 20
	}
	return n
}

func eraiPerAnimeRetryBaseBackoff() time.Duration {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_PER_ANIME_RETRY_BACKOFF"))
	if v == "" {
		return 2 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 2 * time.Second
	}
	return d
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
	Kitsu           *kitsu.Client
	KitsuMinDelay   time.Duration // sleep after each Kitsu enrichment; 0 → default 400ms (client also paces)
	TMDB            *tmdb.Client
	TMDBMinDelay    time.Duration // sleep after each TMDB call; 0 → default 250ms
	SynopsisTrans   ports.SynopsisTranslator // optional: nil in tests; production passes gilang translator
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
	kitsu         *kitsu.Client
	kitsuDelay    time.Duration
	tmdb          *tmdb.Client
	tmdbDelay     time.Duration
	synopsisTrans ports.SynopsisTranslator
	log                 *slog.Logger
	mu                  sync.Mutex
	syncRunning         atomic.Bool
	syncRunStartedUnix  atomic.Int64 // Unix nano UTC while Run holds the lock; 0 when idle
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
	kdly := o.KitsuMinDelay
	if kdly <= 0 {
		kdly = 400 * time.Millisecond
	}
	tmdly := o.TMDBMinDelay
	if tmdly <= 0 {
		tmdly = 250 * time.Millisecond
	}
	return &RSSSyncService{
		repo:          repo,
		mem:           mem,
		getter:        httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, o.MaxBodyBytes),
		anilist:       o.AniList,
		anilistDelay:  dly,
		jikan:         o.Jikan,
		jikanDelay:    jdly,
		kitsu:         o.Kitsu,
		kitsuDelay:    kdly,
		tmdb:          o.TMDB,
		tmdbDelay:     tmdly,
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

// SyncRunStartedAt returns UTC start time of the current Run, or zero if not running.
func (s *RSSSyncService) SyncRunStartedAt() time.Time {
	if s == nil || !s.syncRunning.Load() {
		return time.Time{}
	}
	n := s.syncRunStartedUnix.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
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
			cur := cache[ser.ID]
			// Jikan/MAL na mesma passagem: AniList muitas vezes não tem título alternativo (ex. romanização do RSS).
			if s.jikan != nil && domain.EnrichmentCouldUseJikan(cur) {
				add, jerr := s.jikan.SearchAnimeEnrichment(ctx, ser.Name)
				if jerr == nil {
					merged := domain.MergeAniListEnrichment(cur, add)
					merged.Description = TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
					cache[ser.ID] = merged
					newN++
					s.log.Info("jikan: filled series after anilist miss", slog.String("series", ser.Name), slog.String("search_query", qlog))
					if s.jikanDelay > 0 {
						time.Sleep(s.jikanDelay)
					}
					continue
				}
				s.log.Debug("jikan fallback after anilist failure", slog.String("series", ser.Name), slog.Any("err", jerr))
				if s.kitsu != nil && domain.EnrichmentCouldUseJikan(cur) {
					addK, kerr := s.kitsu.SearchAnimeEnrichment(ctx, ser.Name)
					if kerr == nil {
						merged := domain.MergeAniListEnrichment(cur, addK)
						merged.Description = TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
						cache[ser.ID] = merged
						newN++
						s.log.Info("kitsu: filled series after anilist+jikan miss", slog.String("series", ser.Name), slog.String("search_query", qlog))
						if s.kitsuDelay > 0 {
							time.Sleep(s.kitsuDelay)
						}
						continue
					}
					appendSyncNote(syncNotes, fmt.Sprintf("enrichment %q: anilist: %v; jikan: %v; kitsu: %v", qlog, err, jerr, kerr))
					continue
				}
				appendSyncNote(syncNotes, fmt.Sprintf("enrichment %q: anilist: %v; jikan: %v", qlog, err, jerr))
				continue
			}
			if s.kitsu != nil && domain.EnrichmentCouldUseJikan(cur) {
				addK, kerr := s.kitsu.SearchAnimeEnrichment(ctx, ser.Name)
				if kerr == nil {
					merged := domain.MergeAniListEnrichment(cur, addK)
					merged.Description = TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
					cache[ser.ID] = merged
					newN++
					s.log.Info("kitsu: filled series after anilist miss (no jikan)", slog.String("series", ser.Name), slog.String("search_query", qlog))
					if s.kitsuDelay > 0 {
						time.Sleep(s.kitsuDelay)
					}
					continue
				}
				appendSyncNote(syncNotes, fmt.Sprintf("enrichment %q: anilist: %v; kitsu: %v", qlog, err, kerr))
				continue
			}
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

func (s *RSSSyncService) enrichKitsuGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.kitsu == nil {
		if s.kitsu == nil {
			s.log.Info("kitsu: skipped (set GOANIMES_KITSU_DISABLED=true to disable; no client)")
		}
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("kitsu: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "kitsu: stopped early (context cancelled)")
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
		add, err := s.kitsu.SearchAnimeEnrichment(ctx, ser.Name)
		if err != nil {
			s.log.Debug("kitsu lookup skipped or failed", slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("kitsu %q: %v", ser.Name, err))
			continue
		}
		merged := domain.MergeAniListEnrichment(cur, add)
					merged.Description = TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
		cache[ser.ID] = merged
		n++
		if s.kitsuDelay > 0 {
			time.Sleep(s.kitsuDelay)
		}
	}
	if n > 0 {
		s.log.Info("kitsu: filled gaps", slog.Int("series", n))
	}
}

// enrichKitsuEpisodeMaps merges Kitsu /episodes titles and thumbnails (resolve id by search when missing).
func (s *RSSSyncService) enrichKitsuEpisodeMaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.kitsu == nil {
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("kitsu episodes: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "kitsu episodes: stopped early (context cancelled)")
			return
		default:
		}
		cur := cache[ser.ID]
		kid := strings.TrimSpace(cur.KitsuAnimeID)
		if kid == "" && strings.TrimSpace(ser.Name) != "" {
			id, err := s.kitsu.SearchAnimeID(ctx, ser.Name)
			if err == nil && id != "" {
				cur.KitsuAnimeID = id
				cache[ser.ID] = cur
				kid = id
			}
		}
		if kid == "" {
			continue
		}
		titles, thumbs, err := s.kitsu.FetchEpisodeMaps(ctx, kid)
		if err != nil {
			s.log.Debug("kitsu episodes failed", slog.String("kitsu_id", kid), slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("kitsu episodes %s: %v", kid, err))
			continue
		}
		if len(titles) == 0 && len(thumbs) == 0 {
			continue
		}
		cache[ser.ID] = domain.MergeAniListEnrichment(cache[ser.ID], domain.AniListSeriesEnrichment{
			KitsuAnimeID:          kid,
			EpisodeTitleByNum:     titles,
			EpisodeThumbnailByNum: thumbs,
		})
		n++
		if s.kitsuDelay > 0 {
			time.Sleep(s.kitsuDelay)
		}
	}
	if n > 0 {
		s.log.Info("kitsu: merged episode titles/thumbnails", slog.Int("series", n))
	}
}

// enrichJikanMalEpisodeTitles pulls MAL episode names via Jikan when MalID is known (e.g. AniList idMal),
// without repeating a full Jikan anime search. Merge only fills episode numbers still missing in cache.
func (s *RSSSyncService) enrichJikanMalEpisodeTitles(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.jikan == nil {
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("jikan mal episodes: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "jikan mal episodes: stopped early (context cancelled)")
			return
		default:
		}
		cur := cache[ser.ID]
		if cur.MalID <= 0 {
			continue
		}
		eps, err := s.jikan.FetchEpisodeTitlesByMalID(ctx, cur.MalID)
		if err != nil {
			s.log.Debug("jikan mal episodes failed", slog.Int("mal_id", cur.MalID), slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("jikan mal episodes %d: %v", cur.MalID, err))
			continue
		}
		if len(eps) == 0 {
			continue
		}
		cache[ser.ID] = domain.MergeAniListEnrichment(cur, domain.AniListSeriesEnrichment{EpisodeTitleByNum: eps})
		n++
		if s.jikanDelay > 0 {
			time.Sleep(s.jikanDelay)
		}
	}
	if n > 0 {
		s.log.Info("jikan: merged MAL episode titles by mal_id", slog.Int("series", n))
	}
}

func (s *RSSSyncService) resolveStremioHeroBackgrounds(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 {
		return
	}
	if s.tmdb == nil {
			s.log.Info("tmdb: skipped (no API key or GOANIMES_TMDB_DISABLED; hero uses AniList/Kitsu only)")
	}
	n := 0
	for i, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("stremio hero: stopped early (context done)", slog.Int("resolved", n))
			appendSyncNote(syncNotes, "stremio hero: stopped early (context cancelled)")
			return
		default:
		}
		en := cache[ser.ID]
		search := ser.Name
		var tmdbCands []domain.BackgroundCandidate
		if s.tmdb != nil {
			cands, err := TMDBBackdropCandidatesForEnrichment(ctx, s.tmdb, en, search)
			if err != nil {
				s.log.Debug("tmdb backdrop fetch failed", slog.String("series", ser.Name), slog.Any("err", err))
				appendSyncNote(syncNotes, fmt.Sprintf("tmdb %q: %v", ser.Name, err))
			} else {
				tmdbCands = cands
			}
			if i > 0 && s.tmdbDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.tmdbDelay):
				}
			}
		}
		hero := strings.TrimSpace(domain.ResolveStremioHeroBackground(en, tmdbCands))
		if hero == "" {
			continue
		}
		en.StremioHeroBackgroundURL = hero
		cache[ser.ID] = en
		n++
	}
	if n > 0 {
		s.log.Info("stremio hero: resolved backgrounds", slog.Int("series", n), slog.Bool("tmdb", s.tmdb != nil))
	}
}

// Run fetches all RSS sources and rebuilds the catalog.
func (s *RSSSyncService) Run(ctx context.Context) domain.SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	started := time.Now().UTC()
	s.syncRunStartedUnix.Store(started.UnixNano())
	s.syncRunning.Store(true)
	defer func() {
		s.syncRunning.Store(false)
		s.syncRunStartedUnix.Store(0)
	}()
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
		var emptyNotes []string
		s.resolveStremioHeroBackgrounds(ctx, snap.Series, anilistCache, &emptyNotes)
		snap.AniListBySeries = anilistCache
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
	s.log.Info("erai per-anime rss",
		slog.Int("slugs_queued", len(eraiJobs)),
		slog.Int("fetched", perAnimeOK),
		slog.Int("per_anime_cap", perAnimeCap),
		slog.Duration("per_anime_delay", perAnimeDelay),
		slog.Int("per_anime_max_attempts", perAnimeAttempts),
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
	s.enrichKitsuGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichKitsuEpisodeMaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichJikanMalEpisodeTitles(ctx, snap.Series, anilistCache, &enrichNotes)
	s.resolveStremioHeroBackgrounds(ctx, snap.Series, anilistCache, &enrichNotes)
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
