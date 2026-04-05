package rsssync

import (
	"strings"
	"testing"
)

func TestCapPersistedSyncLines(t *testing.T) {
	long := make([]string, maxPersistedSyncErrors+10)
	for i := range long {
		long[i] = "e"
	}
	out := capPersistedSyncLines(long)
	if len(out) != maxPersistedSyncErrors+1 {
		t.Fatalf("got len %d want %d", len(out), maxPersistedSyncErrors+1)
	}
	if !strings.Contains(out[len(out)-1], "more") {
		t.Fatalf("expected truncation suffix, got %q", out[len(out)-1])
	}
	short := []string{"a", "b"}
	if got := capPersistedSyncLines(short); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("short slice should pass through unchanged, got %#v", got)
	}
}
