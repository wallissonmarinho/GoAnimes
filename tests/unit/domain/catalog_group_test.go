package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestParseEraiReleaseTitle(t *testing.T) {
	sn, ep, sp, ok := domain.ParseEraiReleaseTitle("[Torrent] Chitose-kun wa Ramune Bin no Naka - 13 [720p CR WEB-DL AVC AAC][us][br]")
	require.True(t, ok)
	require.Equal(t, "Chitose-kun wa Ramune Bin no Naka", sn)
	require.Equal(t, 13, ep)
	require.False(t, sp)

	sn, ep, sp, ok = domain.ParseEraiReleaseTitle("[Torrent] Isekai Foo - Special [SD CR WEB-DL AVC AAC][us][br]")
	require.True(t, ok)
	require.Equal(t, "Isekai Foo", sn)
	require.Equal(t, 0, ep)
	require.True(t, sp)

	_, _, _, ok = domain.ParseEraiReleaseTitle("no pattern here")
	require.False(t, ok)
}

func TestEnsureSnapshotGrouped(t *testing.T) {
	snap := &domain.CatalogSnapshot{
		Items: []domain.CatalogItem{
			{ID: "goanimes:a", Name: "[Torrent] Zeta Show - 02 [1080p][br]"},
			{ID: "goanimes:b", Name: "[Torrent] Alpha Show - 01 [720p][br]"},
		},
	}
	domain.EnsureSnapshotGrouped(snap)
	require.Len(t, snap.Series, 2)
	require.Equal(t, "Alpha Show", snap.Series[0].Name)
	require.Equal(t, "Zeta Show", snap.Series[1].Name)
	require.NotEmpty(t, snap.Items[0].SeriesID)
}
