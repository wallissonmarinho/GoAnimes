package rss_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	rssadapter "github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
)

func TestParseFeed_keepsOnlyBRSubtitleItems(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="http://www.erai-raws.info/dtd">
<channel>
<title>Test</title>
<item>
<title>Show A - 01</title>
<link>magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;dn=test</link>
<guid isPermaLink="false">a-guid-1</guid>
<erai:subtitles>[us][mx]</erai:subtitles>
</item>
<item>
<title>Show B - 02</title>
<link>magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&amp;dn=test2</link>
<guid isPermaLink="false">a-guid-2</guid>
<erai:subtitles>[us][br][mx]</erai:subtitles>
</item>
</channel>
</rss>`
	items, err := rssadapter.ParseFeed([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Show B - 02", items[0].Name)
	require.Contains(t, items[0].SubtitlesTag, "[br]")
	require.Equal(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", items[0].InfoHash)
	require.Equal(t, rssadapter.StremioMetaType, items[0].Type)
}

func TestParseFeed_torrentEnclosure(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="http://www.erai-raws.info/dtd">
<channel>
<item>
<title>C - 03</title>
<link>https://example.com/page</link>
<guid>c-guid</guid>
<enclosure url="https://example.com/x.torrent" type="application/x-bittorrent"/>
<erai:subtitles>[br]</erai:subtitles>
</item>
</channel>
</rss>`
	items, err := rssadapter.ParseFeed([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "https://example.com/x.torrent", items[0].TorrentURL)
}
