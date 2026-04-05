#!/usr/bin/env bash
# Fail if RSS sync sources were copied into package services (causes undefined: RSSSyncService in IDE/build).
set -euo pipefail
shopt -s nullglob
dir="internal/core/services"
for f in "$dir"/rss_sync_*.go "$dir"/rss_feed_probe.go; do
	if [[ -f "$f" ]]; then
		echo "check-layout: forbidden file (RSS sync lives only in internal/core/rsssync/):" >&2
		echo "  $f" >&2
		echo "Remove the file or discard the unsaved editor buffer, then reopen internal/core/rsssync/$(basename "$f")" >&2
		exit 1
	fi
done
