package services_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

type stubEpisodeTitleTr struct{}

func (stubEpisodeTitleTr) Name() string { return "stub" }

func (stubEpisodeTitleTr) Translate(text, source, target string) (string, error) {
	_ = source
	_ = target
	if text == "Hello" {
		return "Olá", nil
	}
	return text, nil
}

func TestTranslateEpisodeTitleToPT(t *testing.T) {
	tr := stubEpisodeTitleTr{}
	require.Equal(t, "", services.TranslateEpisodeTitleToPT(tr, nil, ""))
	require.Equal(t, "x", services.TranslateEpisodeTitleToPT(tr, nil, "x"))
	require.Equal(t, "Olá", services.TranslateEpisodeTitleToPT(tr, slog.Default(), "Hello"))
	require.Equal(t, "O primeiro episódio",
		services.TranslateEpisodeTitleToPT(tr, slog.Default(), "O primeiro episódio"))
}
