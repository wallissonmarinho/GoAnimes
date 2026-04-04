package anidb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAnimeEpisodeTitles_sample(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<anime id="1" restricted="false">
  <episodes>
    <episode id="1" update="2011-07-01">
      <epno type="1">1</epno>
      <title xml:lang="ja">侵略</title>
      <title xml:lang="en">Invasion</title>
    </episode>
    <episode id="2" update="2011-07-01">
      <epno type="2">S1</epno>
      <title xml:lang="en">OVA special</title>
    </episode>
  </episodes>
</anime>`
	m, err := parseAnimeEpisodeTitles([]byte(xmlDoc))
	require.NoError(t, err)
	require.Equal(t, "Invasion", m[1])
	require.Len(t, m, 1)
}

func TestParseAnimeEpisodeTitles_error(t *testing.T) {
	_, err := parseAnimeEpisodeTitles([]byte(`<error>Banned</error>`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Banned")
}

func TestParseAnidbAidFromURL(t *testing.T) {
	require.Equal(t, 19614, ParseAnidbAidFromURL("https://anidb.net/anime/19614"))
	require.Equal(t, 0, ParseAnidbAidFromURL(""))
}
