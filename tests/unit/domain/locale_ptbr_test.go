package domain_test

import (
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
