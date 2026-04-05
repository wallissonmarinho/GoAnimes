// Package services provides catalog admin and small shared helpers (synopsis, TMDB hero)
// used by rsssync and stremio.
//
// Do not add RSS sync code here: RSSSyncService and rss_sync_*.go belong only in
// internal/core/rsssync. Duplicate files under this directory (e.g. rss_sync_fetch.go)
// declare package services but use RSSSyncService, which is undefined here — delete them
// or discard unsaved editor buffers.
package services
