package anilist_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
)

func TestNormalizeDescription(t *testing.T) {
	require.Equal(t, "Hello world", anilist.NormalizeDescription("  Hello <b>world</b>  "))
}
