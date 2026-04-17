package rsssync

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCinemetaQueriesForSeries_StripsSeasonNoiseAndAddsNoSeasonVariant(t *testing.T) {
	in := "Diamond no Ace: Act II Second Season - 02 (HEVC) [1080p CR WEBRip HEVC AAC][Encoded]"
	got := cinemetaQueriesForSeries(in)
	require.NotEmpty(t, got)
	require.Contains(t, got, "Diamond no Ace")
}

func TestCinemetaQueriesForSeries_StripsBatchAndVersionNoise(t *testing.T) {
	in := "[Torrent] Meitantei Precure - 01 ~ 10v2 [1080p AMZN WEB-DL AVC EAC3][Batch]"
	got := cinemetaQueriesForSeries(in)
	require.NotEmpty(t, got)
	require.Contains(t, got, "Meitantei Precure")
}
