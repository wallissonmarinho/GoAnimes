package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestAnimeSearchScoringTokens(t *testing.T) {
	require.Equal(t, []string{"dorohedoro"}, domain.AnimeSearchScoringTokens("Dorohedoro Season 2"))
	require.Equal(t, []string{"solo", "leveling"}, domain.AnimeSearchScoringTokens("Solo Leveling Season 2"))
}
