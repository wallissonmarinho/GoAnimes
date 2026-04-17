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

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/cinemeta"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
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
	Cinemeta        *cinemeta.Client
	CinemetaMinDelay time.Duration // sleep after each Cinemeta request; 0 -> default 250ms
	TMDB            *tmdb.Client
	TMDBMinDelay    time.Duration            // sleep after each TMDB call; 0 → default 250ms
	TheTVDB         *thetvdb.Client          // optional: IMDb→TVDB episode titles/thumbnails + fanart hero candidates
	TVDBMinDelay    time.Duration            // sleep after each TheTVDB call; 0 → default 400ms
	SynopsisTrans   ports.SynopsisTranslator // optional: nil in tests; production passes gilang translator
}

// RSSSyncService fetches RSS sources, filters pt-BR (Erai [br]), updates memory + DB snapshot.
type RSSSyncService struct {
	repo               ports.CatalogRepository
	mem                *state.CatalogStore
	getter             *httpclient.Getter
	cinemeta           *cinemeta.Client
	cinemetaDelay      time.Duration
	tmdb               *tmdb.Client
	tmdbDelay          time.Duration
	tvdb               *thetvdb.Client
	tvdbDelay          time.Duration
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
	cmdly := o.CinemetaMinDelay
	if cmdly <= 0 {
		cmdly = 250 * time.Millisecond
	}
	tmdly := o.TMDBMinDelay
	if tmdly <= 0 {
		tmdly = 250 * time.Millisecond
	}
	tvdly := o.TVDBMinDelay
	if tvdly <= 0 {
		tvdly = 400 * time.Millisecond
	}
	return &RSSSyncService{
		repo:          repo,
		mem:           mem,
		getter:        httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, o.MaxBodyBytes),
		cinemeta:      o.Cinemeta,
		cinemetaDelay: cmdly,
		tmdb:          o.TMDB,
		tmdbDelay:     tmdly,
		tvdb:          o.TheTVDB,
		tvdbDelay:     tvdly,
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
	anilistCache := cloneSeriesEnrichmentCache(prevSnap.SeriesEnrichmentBySeriesID)
	live := s.mem.Snapshot()
	for k, v := range live.SeriesEnrichmentBySeriesID {
		anilistCache[k] = domain.MergeSeriesEnrichment(anilistCache[k], v)
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
			SeriesEnrichmentBySeriesID: anilistCache,
			RSSMainFeedBuildByURL: map[string]domain.RssMainFeedBuildFingerprint{},
		}
		domain.EnsureSnapshotGrouped(&snap)
		var emptyNotes []string
		s.resolveStremioHeroBackgrounds(ctx, snap.Series, anilistCache, &emptyNotes)
		snap.SeriesEnrichmentBySeriesID = anilistCache
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
	s.enrichCinemetaGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.enrichTheTVDBGaps(ctx, snap.Series, anilistCache, &enrichNotes)
	s.translateEpisodeTitlesToPT(ctx, snap.Series, anilistCache, &enrichNotes)
	s.resolveStremioHeroBackgrounds(ctx, snap.Series, anilistCache, &enrichNotes)
	snap.SeriesEnrichmentBySeriesID = anilistCache
	pruneSeriesEnrichmentCache(snap.SeriesEnrichmentBySeriesID, snap.Series)
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
