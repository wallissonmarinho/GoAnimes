package rss_test

import (
	"net/url"
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

func TestParseFeed_httpsEraiNamespace_parsesSubtitles(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="https://www.erai-raws.info/rss-page/">
<channel>
<item>
<title>Champignon no Majo - 12 (HEVC)</title>
<link>https://t.erai-raws.info/Torrent/2026/Winter/Champignon/x.torrent</link>
<guid isPermaLink="false">g-torrent</guid>
<erai:subtitles>[us][br][mx]</erai:subtitles>
<erai:infohash>aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa</erai:infohash>
</item>
</channel>
</rss>`
	items, err := rssadapter.ParseFeed([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Contains(t, items[0].SubtitlesTag, "[br]")
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", items[0].InfoHash)
	require.NotEmpty(t, items[0].TorrentURL)
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

func TestEraiSourceOriginAndToken_andBuildPerAnimeFeedURL(t *testing.T) {
	src := "https://www.erai-raws.info/rss-feeds/?type=magnet&quality=1080p&token=abc123def"
	origin, tok := rssadapter.EraiSourceOriginAndToken(src)
	require.Equal(t, "https://www.erai-raws.info", origin)
	require.Equal(t, "abc123def", tok)
	u := rssadapter.BuildEraiPerAnimeFeedURL(origin, "my-show-slug", tok)
	require.Equal(t, "https://www.erai-raws.info/anime-list/my-show-slug/feed/?token=abc123def", u)
}

func TestEraiAnimeListSlugFromEpisodeSlug(t *testing.T) {
	require.Equal(t, "reincarnation-no-kaben", rssadapter.EraiAnimeListSlugFromEpisodeSlug("reincarnation-no-kaben-01"))
	require.Equal(t, "dr-stone-science-future-part-3", rssadapter.EraiAnimeListSlugFromEpisodeSlug("dr-stone-science-future-part-3-01"))
	require.Equal(t, "otonari-no-tenshi-sama-ni-itsunomanika-dame-ningen-ni-sareteita-ken-2",
		rssadapter.EraiAnimeListSlugFromEpisodeSlug("otonari-no-tenshi-sama-ni-itsunomanika-dame-ningen-ni-sareteita-ken-2-01"))
	require.Equal(t, "fangkai-nage-nuwu", rssadapter.EraiAnimeListSlugFromEpisodeSlug("fangkai-nage-nuwu-05v2-chinese-audio"))
	require.Equal(t, "hitori-no-shita-the-outcast-6th-season", rssadapter.EraiAnimeListSlugFromEpisodeSlug("hitori-no-shita-the-outcast-6th-season-14-chinese-audio"))
	require.Equal(t, "darwin-jihen", rssadapter.EraiAnimeListSlugFromEpisodeSlug("darwin-jihen-13-3"))
}

func TestParseFeedWithEraiSlugs_collectsSlugFromEpisodesLinkInDescription(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="http://www.erai-raws.info/dtd">
<channel>
<item>
<title>Show - 01</title>
<link>magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&amp;dn=x</link>
<guid isPermaLink="false">g1</guid>
<description><![CDATA[ <a href="https://www.erai-raws.info/episodes/reincarnation-no-kaben-01/">x</a> ]]></description>
<erai:subtitles>[br]</erai:subtitles>
</item>
</channel>
</rss>`
	items, slugs, err := rssadapter.ParseFeedWithEraiSlugs([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []string{"reincarnation-no-kaben"}, slugs)
}

func TestParseFeedWithEraiSlugs_collectsAnimeListSlugs(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="http://www.erai-raws.info/dtd">
<channel>
<item>
<title>Show - 01</title>
<link>magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&amp;dn=x</link>
<guid isPermaLink="false">g1</guid>
<description><![CDATA[ <a href="https://www.erai-raws.info/anime-list/otonari-no-tenshi/">page</a> ]]></description>
<erai:subtitles>[br]</erai:subtitles>
</item>
</channel>
</rss>`
	items, slugs, err := rssadapter.ParseFeedWithEraiSlugs([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []string{"otonari-no-tenshi"}, slugs)
}

func TestParseFeedWithEraiSlugs_magnetWithXMLAmpersands_findsEncodeSlug(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="https://www.erai-raws.info/rss-page/">
<channel>
<item>
<title>[Magnet] Champignon no Majo - 12 (HEVC) [1080p][us][br]</title>
<link>magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;dn=x&amp;tr=udp%3A%2F%2Ftracker</link>
<description><![CDATA[ <a href="https://www.erai-raws.info/encodes/champignon-no-majo-12-hevc/">mkv</a> ]]></description>
<erai:subtitles>[us][br]</erai:subtitles>
</item>
</channel>
</rss>`
	items, slugs, err := rssadapter.ParseFeedWithEraiSlugs([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", items[0].InfoHash)
	require.Equal(t, []string{"champignon-no-majo"}, slugs)
}

func TestParseFeedWithEraiSlugs_collectsSlugFromEncodesLinkInDescription(t *testing.T) {
	const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:erai="http://www.erai-raws.info/dtd">
<channel>
<item>
<title>Champignon no Majo - 12 (HEVC)</title>
<link>https://t.erai-raws.info/Torrent/2026/Winter/Champignon/x.torrent</link>
<guid isPermaLink="false">g-enc</guid>
<description><![CDATA[ <a href="https://www.erai-raws.info/encodes/champignon-no-majo-12-hevc/">mkv</a> ]]></description>
<erai:subtitles>[us][br][mx]</erai:subtitles>
</item>
</channel>
</rss>`
	items, slugs, err := rssadapter.ParseFeedWithEraiSlugs([]byte(xmlDoc))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []string{"champignon-no-majo"}, slugs)
}

// Champignon no Majo (encode RSS item): slug a partir de /encodes/… e URL per-anime igual à que a Erai usa (token fictício).
func TestChampignonNoMajo_encodeItem_generatesExpectedPerAnimeFeedURL(t *testing.T) {
	const ficticioToken = "ficticio_token_xyz"
	const wantFeedURL = "https://www.erai-raws.info/anime-list/champignon-no-majo/feed/?token=ficticio_token_xyz"

	const descSnippet = `<a href="https://www.erai-raws.info/encodes/champignon-no-majo-12-hevc/">[Erai-raws] Champignon no Majo - 12</a> | Subtitles: [us][br]`
	slugs := rssadapter.ExtractEraiAnimeListSlugsFromEncodesLinks(descSnippet)
	require.Equal(t, []string{"champignon-no-majo"}, slugs)

	origin, tok := rssadapter.EraiSourceOriginAndToken(
		"https://www.erai-raws.info/rss-feeds/?type=torrent&token=" + ficticioToken)
	require.Equal(t, "https://www.erai-raws.info", origin)
	require.Equal(t, ficticioToken, tok)

	got := rssadapter.BuildEraiPerAnimeFeedURL(origin, slugs[0], tok)
	require.Equal(t, wantFeedURL, got, "must match Erai pattern …/anime-list/{slug}/feed/?token=")

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "/anime-list/champignon-no-majo/feed/", parsed.Path)
	require.Equal(t, ficticioToken, parsed.Query().Get("token"))
}
