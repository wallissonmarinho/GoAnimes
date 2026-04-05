package rsssync

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// ingestRSSMainFeedProbe records the main feed fingerprint after a successful download in Run (conditional GET for poll).
func (s *RSSSyncService) ingestRSSMainFeedProbe(feedURL string, body []byte, hdr http.Header) {
	u := strings.TrimSpace(feedURL)
	if u == "" || len(body) == 0 || hdr == nil {
		return
	}
	s.rssProbeMu.Lock()
	defer s.rssProbeMu.Unlock()
	if s.rssProbeByURL == nil {
		s.rssProbeByURL = make(map[string]rssProbeState)
	}
	s.rssProbeByURL[u] = rssProbeStateFromResponse(body, hdr)
}

func (s *RSSSyncService) rssMainFeedBuildForPersist(sources []domain.RSSSource, prevSnap domain.CatalogSnapshot) map[string]domain.RssMainFeedBuildFingerprint {
	out := make(map[string]domain.RssMainFeedBuildFingerprint)
	s.rssProbeMu.Lock()
	defer s.rssProbeMu.Unlock()
	for _, src := range sources {
		u := strings.TrimSpace(src.URL)
		if u == "" {
			continue
		}
		if st, ok := s.rssProbeByURL[u]; ok && st.sha256hex != "" {
			out[u] = domain.RssMainFeedBuildFingerprint{
				ETag: st.etag, LastModified: st.lastModified, SHA256Hex: st.sha256hex,
			}
		} else if prevSnap.RSSMainFeedBuildByURL != nil {
			if prev, ok := prevSnap.RSSMainFeedBuildByURL[u]; ok && prev.SHA256Hex != "" {
				out[u] = prev
			}
		}
	}
	return out
}

func (s *RSSSyncService) ensureRSSLastBuildLoaded(ctx context.Context) {
	s.rssLastBuildOnce.Do(func() {
		s.rssLastBuildMu.Lock()
		defer s.rssLastBuildMu.Unlock()
		s.rssLastBuildByURL = make(map[string]domain.RssMainFeedBuildFingerprint)
		if s.mem != nil {
			live := s.mem.Snapshot()
			for k, v := range live.RSSMainFeedBuildByURL {
				if v.SHA256Hex != "" {
					s.rssLastBuildByURL[k] = v
				}
			}
		}
		if len(s.rssLastBuildByURL) == 0 && s.repo != nil {
			snap, err := s.repo.LoadCatalogSnapshot(ctx)
			if err == nil {
				for k, v := range snap.RSSMainFeedBuildByURL {
					if v.SHA256Hex != "" {
						s.rssLastBuildByURL[k] = v
					}
				}
			}
		}
	})
}

func (s *RSSSyncService) refreshRSSLastBuildFromMem() {
	if s.mem == nil {
		return
	}
	live := s.mem.Snapshot()
	s.rssLastBuildMu.Lock()
	defer s.rssLastBuildMu.Unlock()
	if len(live.RSSMainFeedBuildByURL) == 0 {
		return
	}
	s.rssLastBuildByURL = maps.Clone(live.RSSMainFeedBuildByURL)
}

func (s *RSSSyncService) rssLastBuildHasAnyFingerprint() bool {
	s.rssLastBuildMu.RLock()
	defer s.rssLastBuildMu.RUnlock()
	for _, v := range s.rssLastBuildByURL {
		if v.SHA256Hex != "" {
			return true
		}
	}
	return false
}

// RSSMainFeedsChanged compares live main RSS URLs to the last persisted catalog build (SHA-256 of feed body).
func (s *RSSSyncService) RSSMainFeedsChanged(ctx context.Context) bool {
	if s == nil || s.repo == nil || s.getter == nil {
		return false
	}
	if s.syncRunning.Load() {
		return false
	}
	s.ensureRSSLastBuildLoaded(ctx)

	sources, err := s.repo.ListRSSSources(ctx)
	if err != nil || len(sources) == 0 {
		return false
	}

	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return false
		}
		u := strings.TrimSpace(src.URL)
		if u == "" {
			continue
		}
		seen[u] = struct{}{}

		s.rssProbeMu.Lock()
		if s.rssProbeByURL == nil {
			s.rssProbeByURL = make(map[string]rssProbeState)
		}
		prevProbe := s.rssProbeByURL[u]
		s.rssProbeMu.Unlock()
		if prevProbe.etag == "" && prevProbe.lastModified == "" {
			s.rssLastBuildMu.RLock()
			lb, ok := s.rssLastBuildByURL[u]
			s.rssLastBuildMu.RUnlock()
			if ok && lb.SHA256Hex != "" {
				prevProbe = rssProbeStateFromFingerprint(lb)
			}
		}

		body, status, hdr, gerr := s.getter.GetBytesGETRetryWithHeaders(ctx, u, 3, 2*time.Second, prevProbe.etag, prevProbe.lastModified)
		if gerr != nil {
			if s.log != nil {
				s.log.Debug("rss poll", slog.String("url", u), slog.Any("err", gerr))
			}
			return false
		}

		if status == http.StatusNotModified {
			s.rssProbeMu.Lock()
			if cur := s.rssProbeByURL[u]; cur.sha256hex == "" {
				s.rssLastBuildMu.RLock()
				lb, ok := s.rssLastBuildByURL[u]
				s.rssLastBuildMu.RUnlock()
				if ok && lb.SHA256Hex != "" {
					s.rssProbeByURL[u] = rssProbeStateFromFingerprint(lb)
				}
			}
			s.rssProbeMu.Unlock()
			continue
		}
		if status != http.StatusOK || len(body) == 0 || hdr == nil {
			return false
		}
		next := rssProbeStateFromResponse(body, hdr)

		s.rssProbeMu.Lock()
		s.rssProbeByURL[u] = next
		s.rssProbeMu.Unlock()
	}

	// No persisted baseline yet (upgrade / first install): adopt current feeds without triggering sync.
	if !s.rssLastBuildHasAnyFingerprint() {
		adopted := make(map[string]domain.RssMainFeedBuildFingerprint)
		allOK := true
		for _, src := range sources {
			u := strings.TrimSpace(src.URL)
			if u == "" {
				continue
			}
			s.rssProbeMu.Lock()
			st, ok := s.rssProbeByURL[u]
			s.rssProbeMu.Unlock()
			if !ok || st.sha256hex == "" {
				allOK = false
				break
			}
			adopted[u] = domain.RssMainFeedBuildFingerprint{
				ETag: st.etag, LastModified: st.lastModified, SHA256Hex: st.sha256hex,
			}
		}
		if allOK && len(adopted) == len(seen) {
			s.rssLastBuildMu.Lock()
			s.rssLastBuildByURL = adopted
			s.rssLastBuildMu.Unlock()
			if s.log != nil {
				s.log.Debug("rss poll: adopted baseline fingerprints (no prior build metadata); next full sync will persist")
			}
		}
		return false
	}

	changed := false
	s.rssLastBuildMu.RLock()
	for _, src := range sources {
		u := strings.TrimSpace(src.URL)
		if u == "" {
			continue
		}
		s.rssProbeMu.Lock()
		cur, ok := s.rssProbeByURL[u]
		s.rssProbeMu.Unlock()
		if !ok || cur.sha256hex == "" {
			changed = true
			break
		}
		last, had := s.rssLastBuildByURL[u]
		if !had || last.SHA256Hex == "" {
			changed = true
			break
		}
		if last.SHA256Hex != cur.sha256hex {
			changed = true
			break
		}
	}
	if !changed {
		for k := range s.rssLastBuildByURL {
			if _, ok := seen[k]; !ok {
				changed = true
				break
			}
		}
	}
	s.rssLastBuildMu.RUnlock()

	s.rssProbeMu.Lock()
	for k := range s.rssProbeByURL {
		if _, ok := seen[k]; !ok {
			delete(s.rssProbeByURL, k)
		}
	}
	s.rssProbeMu.Unlock()

	return changed
}
