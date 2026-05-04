package sync_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/app/sync"
)

func TestNormalizeTitle_extractsAnimeNameAndEpisode(t *testing.T) {
	// Standard format - removes episode and quality markers, lowercases, normalizes spaces
	// Note: keeps other text like "Multiple Subtitle" unless in brackets
	name, ep, quality := sync.NormalizeTitle("Erai-raws Solo Leveling - 01 1080p Multiple Subtitle")
	require.Equal(t, "erai-raws solo leveling multiple subtitle", name)
	require.Equal(t, 1, ep)
	require.Equal(t, "1080p", quality)

	// Another standard format with episode dash notation
	name, ep, quality = sync.NormalizeTitle("Erai-raws Jujutsu Kaisen S2 - 12 1080p HEVC Multiple Subtitle")
	require.NotEmpty(t, name)
	require.Equal(t, 12, ep)
	require.Equal(t, "1080p", quality)

	// 720p quality
	name, ep, quality = sync.NormalizeTitle("Erai-raws Chainsaw Man - 05 720p Multiple Subtitle")
	require.NotEmpty(t, name)
	require.Equal(t, 5, ep)
	require.Equal(t, "720p", quality)
}

func TestNormalizeTitle_handlesVariousFormats(t *testing.T) {
	// Format with quality and codec tag outside brackets
	name, ep, quality := sync.NormalizeTitle("Erai-raws Dr.STONE 3rd Season - 09 HEVC 1080p CR WEBRip AAC")
	require.NotEmpty(t, name)
	require.Equal(t, 9, ep)
	require.Equal(t, "1080p", quality)

	// Format with title in parentheses (will be removed)
	name, ep, quality = sync.NormalizeTitle("(Some Producer) Title - 02 1080p")
	require.NotEmpty(t, name)
	require.Equal(t, 2, ep)
	require.Equal(t, "1080p", quality)
}

func TestNormalizeTitle_lowercase(t *testing.T) {
	// Should normalize to lowercase for consistency
	name1, _, _ := sync.NormalizeTitle("SOLO LEVELING - 01 1080p")
	name2, _, _ := sync.NormalizeTitle("Solo Leveling - 01 1080p")
	name3, _, _ := sync.NormalizeTitle("solo leveling - 01 1080p")

	require.Equal(t, name1, name2)
	require.Equal(t, name2, name3)
	require.Equal(t, "solo leveling", name1)
}

func TestNormalizeTitle_seasonFormatVariations(t *testing.T) {
	// Season 2
	name, ep, _ := sync.NormalizeTitle("Show Name Season 2 - 03 1080p")
	require.Equal(t, "show name season 2", name)
	require.Equal(t, 3, ep)

	// S2 format
	name, ep, _ = sync.NormalizeTitle("Show Name S2 - 04 1080p")
	require.Equal(t, "show name s2", name)
	require.Equal(t, 4, ep)

	// Part 2
	name, ep, _ = sync.NormalizeTitle("Show Name Part 2 - 05 1080p")
	require.Equal(t, "show name part 2", name)
	require.Equal(t, 5, ep)
}

func TestNormalizeTitle_extractsEpisodeNumber(t *testing.T) {
	// Single digit episode
	_, ep, _ := sync.NormalizeTitle("Show - 01 1080p")
	require.Equal(t, 1, ep)

	// Double digit episode
	_, ep, _ = sync.NormalizeTitle("Show - 12 1080p")
	require.Equal(t, 12, ep)

	// Three digit episode
	_, ep, _ = sync.NormalizeTitle("Show - 123 1080p")
	require.Equal(t, 123, ep)

	// Episode with decimal (should floor)
	_, ep, _ = sync.NormalizeTitle("Show - 10.5 1080p")
	require.Equal(t, 10, ep)
}

func TestNormalizeTitle_extractsQuality(t *testing.T) {
	// 1080p
	_, _, quality := sync.NormalizeTitle("Show - 01 1080p")
	require.Equal(t, "1080p", quality)

	// 720p
	_, _, quality = sync.NormalizeTitle("Show - 01 720p")
	require.Equal(t, "720p", quality)

	// 480p
	_, _, quality = sync.NormalizeTitle("Show - 01 480p")
	require.Equal(t, "480p", quality)

	// 2160p
	_, _, quality = sync.NormalizeTitle("Show - 01 2160p")
	require.Equal(t, "2160p", quality)

	// No quality tag (empty)
	_, _, quality = sync.NormalizeTitle("Show - 01")
	require.Equal(t, "", quality)
}

func TestNormalizeTitle_stripsSourceTags(t *testing.T) {
	// Should strip source info in brackets
	name, _, _ := sync.NormalizeTitle("Show [us] [br] - 01 1080p")
	require.Equal(t, "show", name)

	// Should not include codec or container info in name
	name, _, _ = sync.NormalizeTitle("Show CR WEBRip HEVC AAC - 01 1080p")
	require.Equal(t, "show cr webrip hevc aac", name)
}

func TestNormalizeTitle_handlesEdgeCases(t *testing.T) {
	// Empty string
	name, ep, quality := sync.NormalizeTitle("")
	require.Equal(t, "", name)
	require.Equal(t, 0, ep)
	require.Equal(t, "", quality)

	// No episode number
	name, ep, quality = sync.NormalizeTitle("Show Title 1080p")
	require.Equal(t, "show title", name)
	require.Equal(t, 0, ep)

	// Malformed
	name, ep, quality = sync.NormalizeTitle("Random Torrent Title")
	require.NotEmpty(t, name)
	require.Equal(t, 0, ep)
}
