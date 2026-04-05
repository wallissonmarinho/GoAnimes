package rsssync

import "fmt"

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
