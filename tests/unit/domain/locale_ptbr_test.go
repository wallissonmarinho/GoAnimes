package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestTranslateAnimeGenresToPTBR(t *testing.T) {
	require.Equal(t, []string{"Ação", "Comédia", "Fantasia", "Slice of life"},
		domain.TranslateAnimeGenresToPTBR([]string{"Action", "Comedy", "fantasy", "Slice of Life"}))
	require.Nil(t, domain.TranslateAnimeGenresToPTBR(nil))
}

func TestLocalizeAniListDescriptionPTBR(t *testing.T) {
	in := "Hello world. (Source: Crunchyroll)"
	require.Equal(t, "Hello world. (Fonte: Crunchyroll)", domain.LocalizeAniListDescriptionPTBR(in))
	require.Equal(t, "", domain.LocalizeAniListDescriptionPTBR("  "))
}

func TestSplitSynopsisBodyAndAttribution(t *testing.T) {
	body, attr := domain.SplitSynopsisBodyAndAttribution("The hero wins. (Fonte: Crunchyroll)")
	require.Equal(t, "The hero wins.", body)
	require.Equal(t, "(Fonte: Crunchyroll)", attr)
	body, attr = domain.SplitSynopsisBodyAndAttribution("No attribution here")
	require.Equal(t, "No attribution here", body)
	require.Equal(t, "", attr)
	require.Equal(t, "x (y)", domain.JoinSynopsisBodyAndAttribution("x", "(y)"))
	require.Equal(t, "x", domain.JoinSynopsisBodyAndAttribution("x", ""))
}

func TestSynopsisBodyLooksEnglish(t *testing.T) {
	require.True(t, domain.SynopsisBodyLooksEnglish("Having spent his childhood in the slums, he now enjoys peace."))
	require.False(t, domain.SynopsisBodyLooksEnglish("Curto demais"))
	// Proper-noun-heavy AniList English without "the/and/…" should still be treated as English for translation.
	tetsuo := "Tetsuo Yukio returns to frozen Earth. Shogakukan military arc. Yukio stands alone against ice."
	require.False(t, domain.SynopsisBodyLooksEnglish("Tetsuo short."))
	require.True(t, domain.SynopsisBodyLooksEnglish(tetsuo))
	// Already pt-BR: do not flag as English.
	require.False(t, domain.SynopsisBodyLooksEnglish("Tetsuo retorna à Terra congelada. É uma história sobre robôs gigantes. Muito emocionante."))
	// Long Latin text without common English tokens (fallback path).
	require.True(t, domain.SynopsisBodyLooksEnglish(strings.Repeat("Zorblax Vexnor Klympt. ", 6)))
}
