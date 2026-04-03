package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestContainsJapaneseScript(t *testing.T) {
	require.True(t, domain.ContainsJapaneseScript("試験"))
	require.True(t, domain.ContainsJapaneseScript("ひらがな"))
	require.True(t, domain.ContainsJapaneseScript("カタカナ"))
	require.False(t, domain.ContainsJapaneseScript("Chainsaw Man"))
	require.False(t, domain.ContainsJapaneseScript(""))
}
