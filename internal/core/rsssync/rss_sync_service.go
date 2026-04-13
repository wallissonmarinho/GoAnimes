package rsssync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anidb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/jikan"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/kitsu"
	rssadapter "github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/thetvdb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// rssProbeState holds conditional GET headers + body hash for RSS main-feed polling.
type rssProbeState struct {
	etag         string
	lastModified string
	sha256hex    string
}

func rssProbeStateFromResponse(body []byte, hdr http.Header) rssProbeState {
	st := rssProbeState{
		etag:         strings.TrimSpace(hdr.Get("ETag")),
		lastModified: strings.TrimSpace(hdr.Get("Last-Modified")),
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		st.sha256hex = hex.EncodeToString(sum[:])
	}
	return st
}

func rssProbeStateFromFingerprint(f domain.RssMainFeedBuildFingerprint) rssProbeState {
	return rssProbeState{
		etag:         f.ETag,
		lastModified: f.LastModified,
		sha256hex:    f.SHA256Hex,
	}
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
	TMDBMinDelay    time.Duration            // sleep after each TMDB call; 0 → default 250ms
	TheTVDB         *thetvdb.Client          // optional: IMDb→TVDB episode titles/thumbnails + fanart hero candidates
	TVDBMinDelay    time.Duration            // sleep after each TheTVDB call; 0 → default 400ms
	AniDB           *anidb.Client            // optional: registered HTTP API client; episode titles from request=anime
	AniDBMinDelay   time.Duration            // extra sleep after each AniDB success; 0 = client pace only (~2.1s)
	SynopsisTrans   ports.SynopsisTranslator // optional: nil in tests; production passes gilang translator
}

// RSSSyncService fetches RSS sources, filters pt-BR (Erai [br]), updates memory + DB snapshot.
type RSSSyncService struct {
	repo               ports.CatalogRepository
	mem                *state.CatalogStore
	getter             *httpclient.Getter
	anilist            *anilist.Client
	anilistDelay       time.Duration
	jikan              *jikan.Client
	jikanDelay         time.Duration
	kitsu              *kitsu.Client
	kitsuDelay         time.Duration
	tmdb               *tmdb.Client
	tmdbDelay          time.Duration
	tvdb               *thetvdb.Client
	tvdbDelay          time.Duration
	anidb              *anidb.Client
	anidbDelay         time.Duration
	synopsisTrans      ports.SynopsisTranslator
	log                *slog.Logger
	mu                 sync.Mutex
	syncRunning        atomic.Bool
	syncRunStartedUnix atomic.Int64 // Unix nano UTC while Run holds the lock; 0 when idle
	rssProbeMu         sync.Mutex
	rssProbeByURL      map[string]rssProbeState // trimmed feed URL → last GET (conditional headers for poll)
	rssLastBuildMu     sync.RWMutex
	rssLastBuildByURL  map[string]domain.RssMainFeedBuildFingerprint // last persisted main-feed bodies (RSS poll baseline)
	rssLastBuildOnce   sync.Once
}

type goaiReleaseAuditOverrideApplier interface {
	ApplyGoaiReleaseAuditOverrides(ctx context.Context, items []domain.CatalogItem) ([]domain.CatalogItem, error)
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
	tvdly := o.TVDBMinDelay
	if tvdly <= 0 {
		tvdly = 400 * time.Millisecond
	}
	adDly := o.AniDBMinDelay
	if adDly < 0 {
		adDly = 0
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
		tvdb:          o.TheTVDB,
		tvdbDelay:     tvdly,
		anidb:         o.AniDB,
		anidbDelay:    adDly,
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
			OK:                    true,
			Message:               "no rss sources configured",
			ItemCount:             0,
			StartedAt:             started,
			FinishedAt:            time.Now().UTC(),
			Items:                 nil,
			AniListBySeries:       anilistCache,
			RSSMainFeedBuildByURL: map[string]domain.RssMainFeedBuildFingerprint{},
		}
		domain.EnsureSnapshotGrouped(&snap)
		var emptyNotes []string
		s.resolveStremioHeroBackgrounds(ctx, snap.Series, anilistCache, &emptyNotes)
		snap.AniListBySeries = anilistCache
		domain.ApplyAniListEnrichmentToSeries(&snap)
		snap.LastSyncErrors = nil
		if err := s.mem.SetAndPersist(ctx, s.repo, snap); err == nil {
			s.refreshRSSLastBuildFromMem()
		}
		return domain.SyncResult{Message: snap.Message}
	}

	merged, errs := s.collectMergedCatalogItems(ctx, sources, prevSnap, defaultEraiToken)
	if applier, ok := s.repo.(goaiReleaseAuditOverrideApplier); ok {
		adjusted, err := applier.ApplyGoaiReleaseAuditOverrides(ctx, merged)
		if err != nil {
			errs = append(errs, "apply goai release overrides: "+err.Error())
			s.log.Error("goai release overrides", slog.Any("err", err))
		} else {
			merged = adjusted
		}
	}

	snap := domain.CatalogSnapshot{
		OK:                    true,
		ItemCount:             len(merged),
		StartedAt:             started,
		FinishedAt:            time.Now().UTC(),
		Items:                 merged,
		RSSMainFeedBuildByURL: s.rssMainFeedBuildForPersist(sources, prevSnap),
	}
	domain.EnsureSnapshotGrouped(&snap)
	domain.SortCatalogItemsInPlace(snap.Items)
	var enrichNotes []string
	s.enrichAniListSeries(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichJikanGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichKitsuGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichKitsuEpisodeMaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichJikanMalEpisodeTitles(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichAniDBEpisodeTitles(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichTheTVDBGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.translateEpisodeTitlesToPT(ctx, snap.Series, anilistCache, &enrichNotes)
	s.resolveStremioHeroBackgrounds(ctx, snap.Series, anilistCache, &enrichNotes)
	snap.AniListBySeries = anilistCache
	domain.MergeSnapshotSeriesBySharedMalID(&snap)
	pruneAniListCache(snap.AniListBySeries, snap.Series)
	domain.ApplyAniListEnrichmentToSeries(&snap)
	snap.Message = fmt.Sprintf("synced %d episodes in %d series from %d feed(s)", len(merged), len(snap.Series), len(sources))
	snap.LastSyncErrors = capPersistedSyncLines(append(append([]string{}, errs...), enrichNotes...))
	if saveErr := s.mem.SetAndPersist(ctx, s.repo, snap); saveErr != nil {
		errs = append(errs, "save snapshot: "+saveErr.Error())
		s.log.Error("save snapshot", slog.Any("err", saveErr))
		snap.LastSyncErrors = capPersistedSyncLines(append(snap.LastSyncErrors, errs[len(errs)-1]))
		s.mem.Set(snap)
	} else {
		s.refreshRSSLastBuildFromMem()
	}
	return domain.SyncResult{Message: snap.Message, Errors: errs}
}
