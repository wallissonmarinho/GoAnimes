package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestNormalizeExternalAnimeSearchQuery(t *testing.T) {
	require.Equal(t, "Chitose-kun wa Ramune Bin no Naka",
		domain.NormalizeExternalAnimeSearchQuery("[Magnet] Chitose-kun wa Ramune Bin no Naka"))
	require.Equal(t, "Darwin Jihen",
		domain.NormalizeExternalAnimeSearchQuery("[Magnet] Darwin Jihen"))
	require.Equal(t, "Show Title",
		domain.NormalizeExternalAnimeSearchQuery("[Magnet] [720p] Show Title"))
	require.Equal(t, `Hime-sama "Goumon" no Jikan desu 2nd Season`,
		domain.NormalizeExternalAnimeSearchQuery(`[Magnet] Hime-sama \"Goumon\" no Jikan desu 2nd Season`))
	require.Equal(t, "", domain.NormalizeExternalAnimeSearchQuery("   "))
}
