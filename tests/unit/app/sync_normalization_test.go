package sync_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/app/sync"
)

func TestNormalizeTitle_extractsAnimeNameAndEpisode(t *testing.T) {
	// Standard format - removes episode and quality markers, lowercases, normalizes spaces
	// Note: strips known provider prefixes and release-tech noise.
	name, ep, quality := sync.NormalizeTitle("Erai-raws Solo Leveling - 01 1080p Multiple Subtitle")
	require.Equal(t, "solo leveling multiple subtitle", name)
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

func TestNormalizeTitle_seasonEpisodeFormat(t *testing.T) {
	// s01e05 format (common in anime torrents)
	name, ep, _ := sync.NormalizeTitle("Even A Replica Can Fall In Love s01e05 CR WEB-DL AAC2.0 H.264")
	require.Equal(t, "even a replica can fall in love", name)
	require.Equal(t, 5, ep)

	// s01e05 with mixed case
	name, ep, _ = sync.NormalizeTitle("Liar Game S01E05 CR WEB-DL AAC2.0 H.264")
	require.NotEmpty(t, name)
	require.Equal(t, 5, ep)

	// s##e## in middle of long title with codecs
	name, ep, _ = sync.NormalizeTitle("[ToonsHub] LIAR GAME S01E05 1080p CR WEB-DL AAC2.0 H.264 (Multi-Subs)")
	require.NotEmpty(t, name)
	require.Equal(t, 5, ep)

	// Different season
	_, ep, _ = sync.NormalizeTitle("Anime Title s02e12 1080p")
	require.Equal(t, 12, ep)

	// Single digit season and episode
	_, ep, _ = sync.NormalizeTitle("Show s1e3 720p")
	require.Equal(t, 3, ep)
}

func TestNormalizeTitle_stripsSourceTags(t *testing.T) {
	// Should strip source info in brackets
	name, _, _ := sync.NormalizeTitle("Show [us] [br] - 01 1080p")
	require.Equal(t, "show", name)

	// Should not include codec or container info in name
	name, _, _ = sync.NormalizeTitle("Show CR WEBRip HEVC AAC - 01 1080p")
	require.Equal(t, "show", name)
}

func TestNormalizeTitle_toonshubTagsAndTechNoise(t *testing.T) {
	name, ep, quality := sync.NormalizeTitle("[ToonsHub] Witch Hat Atelier S01E06 1080p NF WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=ja;S=en,zhhant,id,ja,ko,ms,th,vi;}")
	require.Equal(t, "witch hat atelier", name)
	require.Equal(t, 6, ep)
	require.Equal(t, "1080p", quality)

	name, ep, quality = sync.NormalizeTitle("[ToonsHub] Witch Hat Atelier S01E06 1080p CR WEB-DL MULTi AAC2.0 H.264 (Multi-Audio, Multi-Subs) {Tags:L0;V9;C1;A=ja,en,ar,frfr,de,it,ptbr,eses,es419;S=en,ar,frfr,de,it,ptbr,ru,es419,eses;}")
	require.Equal(t, "witch hat atelier", name)
	require.Equal(t, 6, ep)
	require.Equal(t, "1080p", quality)
}

func TestNormalizeTitle_realWorldUnmatchedNoise(t *testing.T) {
	name, ep, quality := sync.NormalizeTitle("[ToonsHub] Mission Yozakura Family S02E02 REPACK 1080p DSNP WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=ja;S=en,cs,da,nl,fi,frfr,de,el,hu,it,no,pl,ptbr,ptpt,ro,sk,es419,eses,sv,tr;}")
	require.Equal(t, "mission yozakura family", name)
	require.Equal(t, 2, ep)
	require.Equal(t, "1080p", quality)

	name, ep, quality = sync.NormalizeTitle("[ToonsHub] NIPPON SANGOKU The Three Nations of the Crimson Sun S01E06 1080p AMZN WEB-DL DUAL DDP2.0 H.264 (Dual-Audio, Multi-Subs) {Tags:L0;V9;C1;A=ja,en;S=en,ar,zhhans,zhhant,cs,da,nl,fi,frfr,de,el,he,hi,hu,id,it,ja,ko,ms,pl,ptbr,ptpt,ro,es419,eses,sv,th,tr;}")
	require.Equal(t, "nippon sangoku the three nations of the crimson sun", name)
	require.Equal(t, 6, ep)
	require.Equal(t, "1080p", quality)

	name, ep, quality = sync.NormalizeTitle("[ToonsHub] NIPPON SANGOKU The Three Nations of the Crimson Sun S01E06 1080p AMZN WEB-DL DUAL DDP2.0 H.265 (Dual-Audio, Multi-Subs) {Tags:L0;V9;C2;A=ja,en;S=en,ar,zhhans,zhhant,cs,da,nl,fi,frfr,de,el,he,hi,hu,id,it,ja,ko,ms,pl,ptbr,ptpt,ro,es419,eses,sv,th,tr;}")
	require.Equal(t, "nippon sangoku the three nations of the crimson sun", name)
	require.Equal(t, 6, ep)
	require.Equal(t, "1080p", quality)

	name, ep, quality = sync.NormalizeTitle("[ToonsHub] Rooster Fighter S01E09 1080p DSNP WEB-DL DUAL AAC2.0 H.264 (Dual-Audio, Multi-Subs) {Tags:L0;V9;C1;A=ja,en;S=en,frfr,de,it,ptbr,es419;}")
	require.Equal(t, "rooster fighter", name)
	require.Equal(t, 9, ep)
	require.Equal(t, "1080p", quality)

	name, ep, quality = sync.NormalizeTitle("[ToonsHub] MAO S01E06 1080p DSNP WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=ja;S=en,ptbr,es419;}")
	require.Equal(t, "mao", name)
	require.Equal(t, 6, ep)
	require.Equal(t, "1080p", quality)

	name, ep, quality = sync.NormalizeTitle("[ToonsHub] How dare you S02E18 1080p iQ WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=zh;S=en,ar,zhhans,zhhant,frfr,de,id,ja,ko,ms,ptbr,es419,th,vi;}")
	require.Equal(t, "how dare you", name)
	require.Equal(t, 18, ep)
	require.Equal(t, "1080p", quality)
}

func TestNormalizeTitle_handlesEdgeCases(t *testing.T) {
	// Empty string
	name, ep, quality := sync.NormalizeTitle("")
	require.Equal(t, "", name)
	require.Equal(t, 0, ep)
	require.Equal(t, "", quality)

	// No episode number
	name, ep, _ = sync.NormalizeTitle("Show Title 1080p")
	require.Equal(t, "show title", name)
	require.Equal(t, 0, ep)

	// Malformed
	name, ep, _ = sync.NormalizeTitle("Random Torrent Title")
	require.NotEmpty(t, name)
	require.Equal(t, 0, ep)
}
