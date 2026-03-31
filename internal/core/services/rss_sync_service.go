package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/btmeta"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	rssadapter "github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// RSSSyncRuntimeOptions configures outbound fetch.
type RSSSyncRuntimeOptions struct {
	HTTPTimeout  time.Duration
	MaxBodyBytes int64
	UserAgent    string
}

// RSSSyncService fetches RSS sources, filters pt-BR (Erai [br]), updates memory + DB snapshot.
type RSSSyncService struct {
	repo   ports.CatalogRepository
	mem    *state.CatalogStore
	getter *httpclient.Getter
	log    *slog.Logger
	mu     sync.Mutex
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
	return &RSSSyncService{
		repo:   repo,
		mem:    mem,
		getter: httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, o.MaxBodyBytes),
		log:    log,
	}
}

// Run fetches all RSS sources and rebuilds the catalog.
func (s *RSSSyncService) Run(ctx context.Context) domain.SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	started := time.Now().UTC()
	sources, err := s.repo.ListRSSSources(ctx)
	if err != nil {
		return domain.SyncResult{Message: "list sources failed", Errors: []string{err.Error()}}
	}
	if len(sources) == 0 {
		snap := domain.CatalogSnapshot{
			OK:         true,
			Message:    "no rss sources configured",
			ItemCount:  0,
			StartedAt:  started,
			FinishedAt: time.Now().UTC(),
			Items:      nil,
		}
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

	// Erai often exposes only a .torrent URL. Stremio must get infoHash (or magnet), not a raw .torrent URL,
	// or playback fails with "unrecognized file format".
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
	snap.Message = fmt.Sprintf("synced %d episodes in %d series from %d feed(s)", len(merged), len(snap.Series), len(sources))
	s.mem.Set(snap)
	if saveErr := s.repo.SaveCatalogSnapshot(ctx, snap); saveErr != nil {
		errs = append(errs, "save snapshot: "+saveErr.Error())
		s.log.Error("save snapshot", slog.Any("err", saveErr))
	}
	return domain.SyncResult{Message: snap.Message, Errors: errs}
}
