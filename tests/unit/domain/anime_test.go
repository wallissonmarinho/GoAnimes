package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

func TestEpisodeAddSourceDedupesEquivalentEraiTorrentAndMagnet(t *testing.T) {
	ep := domain.Episode{}

	added := ep.AddSource(domain.Source{
		Provider:   "Erai",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/Honzuki no Gekokujou/[Erai-raws] Honzuki no Gekokujou S4 - 07 [1080p CR WEB-DL AVC AAC][MultiSub].mkv.torrent",
		Quality:    "1080p CR WEB-DL AVC AAC",
	})
	require.True(t, added)

	added = ep.AddSource(domain.Source{
		Provider:   "Erai Honzuki no Gekokujou: Shisho ni Naru Tame ni wa Shudan wo Erandeiraremasen – Ryushu no Youjo",
		MagnetLink: "magnet:?xt=urn:btih:DFBBFAF0EA5FBD8AEF3C90B29AD1FD3755916948&dn=%5BErai-raws%5D%20Honzuki%20no%20Gekokujou%20S4%20-%2007%20%5B1080p%20CR%20WEB-DL%20AVC%20AAC%5D%5BMultiSub%5D%5BDD0BDDFB%5D.mkv",
		Quality:    "1080p CR WEB-DL AVC AAC",
	})
	require.False(t, added)
	require.Len(t, ep.Sources, 1)
}

func TestEpisodeAddSourceKeepsDifferentQualities(t *testing.T) {
	ep := domain.Episode{}

	added := ep.AddSource(domain.Source{
		Provider:   "Erai",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/Honzuki no Gekokujou/[Erai-raws] Honzuki no Gekokujou S4 - 07 [1080p CR WEB-DL AVC AAC][MultiSub].mkv.torrent",
		Quality:    "1080p CR WEB-DL AVC AAC",
	})
	require.True(t, added)

	added = ep.AddSource(domain.Source{
		Provider:   "Erai",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/Honzuki no Gekokujou/[Erai-raws] Honzuki no Gekokujou S4 - 07 [720p CR WEB-DL AVC AAC][MultiSub].mkv.torrent",
		Quality:    "720p CR WEB-DL AVC AAC",
	})
	require.True(t, added)
	require.Len(t, ep.Sources, 2)
}

func TestEpisodeAddSourceDedupesSameReleaseAcrossDifferentProviders(t *testing.T) {
	ep := domain.Episode{}

	added := ep.AddSource(domain.Source{
		Provider:   "Erai Generic Feed",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/Kami no Niwatsuki Kusunoki-tei/[Erai-raws] Kami no Niwatsuki Kusunoki-tei - 07 [1080p CR WEB-DL AVC AAC][MultiSub].mkv.torrent",
		Quality:    "1080p CR WEB-DL AVC AAC",
	})
	require.True(t, added)

	added = ep.AddSource(domain.Source{
		Provider:   "Some Dedicated Feed Name",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/Kami no Niwatsuki Kusunoki-tei/[Erai-raws] Kami no Niwatsuki Kusunoki-tei - 07 [1080p CR WEB-DL AVC AAC][MultiSub].mkv.torrent",
		Quality:    "1080p CR WEB-DL AVC AAC",
	})
	require.False(t, added)
	require.Len(t, ep.Sources, 1)
}

func TestEpisodeAddSourceDedupesRepeatedBatchReleaseInSameEpisode(t *testing.T) {
	ep := domain.Episode{}

	added := ep.AddSource(domain.Source{
		Provider:   "Erai One Piece",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/One Piece/[Erai-raws] One Piece - 1089 ~ 1104 [1080p][Multiple Subtitle].torrent",
		Quality:    "1080p",
	})
	require.True(t, added)

	added = ep.AddSource(domain.Source{
		Provider:   "Erai",
		MagnetLink: "https://t.erai-raws.info/Torrent/2026/Spring/One Piece/[Erai-raws] One Piece - 1089 ~ 1104 [1080p][Multiple Subtitle].torrent",
		Quality:    "1080p",
	})
	require.False(t, added)
	require.Len(t, ep.Sources, 1)
}
