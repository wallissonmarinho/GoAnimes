package stremio_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/app/stremio"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
)

type fakeCatalogRepo struct {
	anime        domain.Anime
	upsertCalled bool
	upsertAnime  domain.Anime
}

func (f *fakeCatalogRepo) UpsertSeason(ctx context.Context, anime domain.Anime) error {
	f.upsertCalled = true
	f.upsertAnime = anime
	return nil
}
func (f *fakeCatalogRepo) AddEpisodeSource(ctx context.Context, tmdbID, season, episode int, src domain.Source) (bool, error) {
	return false, nil
}
func (f *fakeCatalogRepo) UpdateEpisodeDetails(ctx context.Context, tmdbID, season, episode int, title, overview, stillPath string) error {
	return nil
}
func (f *fakeCatalogRepo) GetByTMDBSeason(ctx context.Context, tmdbID, season int) (domain.Anime, bool, error) {
	return f.anime, true, nil
}
func (f *fakeCatalogRepo) ListByGenre(ctx context.Context, genre string, limit, skip int) ([]domain.Anime, error) {
	return nil, nil
}
func (f *fakeCatalogRepo) ListRecent(ctx context.Context, days, limit, skip int) ([]domain.Anime, error) {
	return nil, nil
}
func (f *fakeCatalogRepo) ListAll(ctx context.Context, limit, skip int) ([]domain.Anime, error) {
	return nil, nil
}
func (f *fakeCatalogRepo) ListGenres(ctx context.Context) ([]string, error) { return nil, nil }
func (f *fakeCatalogRepo) RemoveSourcesByProvider(ctx context.Context, provider string) (int, error) {
	return 0, nil
}

type fakeTMDBClient struct {
	details ports.TMDBSeasonDetails
}

func (f *fakeTMDBClient) SearchSeries(ctx context.Context, query string) (ports.TMDBSearchResult, bool, error) {
	return ports.TMDBSearchResult{}, false, nil
}

func (f *fakeTMDBClient) GetSeasonDetails(ctx context.Context, tmdbID, season int) (ports.TMDBSeasonDetails, error) {
	return f.details, nil
}

func (f *fakeTMDBClient) GetEpisodeDetails(ctx context.Context, tmdbID, season, episode int) (ports.TMDBEpisodeDetails, error) {
	return ports.TMDBEpisodeDetails{}, nil
}

func bencodeString(value string) string {
	return fmt.Sprintf("%d:%s", len(value), value)
}

func TestStreamsConvertsTorrentURLToMagnet(t *testing.T) {
	info := "d6:lengthi12345e4:name" + bencodeString("test") + "12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "e"
	announce := "http://t/ann"
	torrentBytes := []byte("d8:announce" + bencodeString(announce) + "4:info" + info + "e")
	infoHash := sha1.Sum([]byte(info))
	expectedHash := hex.EncodeToString(infoHash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(torrentBytes)
	}))
	defer server.Close()

	service := &stremio.Service{Repo: &fakeCatalogRepo{anime: domain.Anime{
		TMDBID:       288551,
		SeasonNumber: 1,
		Episodes: []domain.Episode{{
			Number: 5,
			Sources: []domain.Source{{
				Provider:   "Erai",
				MagnetLink: server.URL + "/release.torrent",
				Quality:    "1080p CR WEBRip HEVC AAC",
			}},
		}},
	}}}

	streams, err := service.Streams(context.Background(), "tmdb:288551:1:5")
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Equal(t, "Torrent · 1080p", streams[0]["name"])
	require.Equal(t, "Episódio 5 · [Torrent] test", streams[0]["title"])
	require.Equal(t, 0, streams[0]["fileIdx"])
	require.Equal(t, expectedHash, streams[0]["infoHash"])
	hints, ok := streams[0]["behaviorHints"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tmdb:288551:1:5", hints["bingeGroup"])
}

func TestStreamsUsesQualityFromMagnetDnWhenMissing(t *testing.T) {
	service := &stremio.Service{Repo: &fakeCatalogRepo{anime: domain.Anime{
		TMDBID:       300126,
		SeasonNumber: 1,
		Episodes: []domain.Episode{{
			Number: 4,
			Sources: []domain.Source{{
				Provider:   "Erai",
				MagnetLink: "magnet:?xt=urn:btih:94d6baed7ccd6f422dc23a6639b0d3e003696a8b&dn=%5BErai-raws%5D+Liar+Game+-+04+%5B1080p+CR+WEBRip+HEVC+AAC%5D%5BMultiSub%5D%5B73D39D91%5D.mkv",
			}},
		}},
	}}}

	streams, err := service.Streams(context.Background(), "tmdb:300126:1:4")
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Equal(t, "Torrent · 1080p", streams[0]["name"])
	require.Equal(t, "Episódio 4 · [Torrent] [Erai-raws] Liar Game - 04 [1080p CR WEBRip HEVC AAC][MultiSub][73D39D91].mkv", streams[0]["title"])
	require.NotContains(t, streams[0], "url")
}

func TestStreamsNormalizesProviderNameWithSeasonSuffix(t *testing.T) {
	service := &stremio.Service{Repo: &fakeCatalogRepo{anime: domain.Anime{
		TMDBID:       96316,
		SeasonNumber: 1,
		Episodes: []domain.Episode{{
			Number: 48,
			Sources: []domain.Source{{
				Provider:   "Erai Kanojo Okarishimasu 4nd Season",
				MagnetLink: "magnet:?xt=urn:btih:94d6baed7ccd6f422dc23a6639b0d3e003696a8b&dn=%5BErai-raws%5D+Kanojo+Okarishimasu+4th+Season+-+12+%5B1080p+CR+WEBRip+HEVC+AAC%5D%5BMultiSub%5D.mkv",
			}},
		}},
	}}}

	streams, err := service.Streams(context.Background(), "tmdb:96316:1:48")
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Equal(t, "Torrent · 1080p", streams[0]["name"])
	require.Equal(t, "Episódio 48 · [Torrent] [Erai-raws] Kanojo Okarishimasu 4th Season - 12 [1080p CR WEBRip HEVC AAC][MultiSub].mkv", streams[0]["title"])
}

func TestMetaIncludesExpandedFieldsFromCatalog(t *testing.T) {
	service := &stremio.Service{
		Repo: &fakeCatalogRepo{anime: domain.Anime{
			TMDBID:       96316,
			SeasonNumber: 1,
			Title:        "Rent-a-Girlfriend",
			AnimeType:    "TV",
			Slug:         "rent-a-girlfriend",
			Aliases:      []string{"Rent-a-Girlfriend", "彼女、お借りします"},
			LogoPath:     "https://image.tmdb.org/t/p/w780/logo.png",
			ReleaseInfo:  "2020-",
			Year:         "2020-",
			Status:       "current",
			Runtime:      "24 min",
			Rating:       8.3,
			PosterPath:   "https://image.tmdb.org/t/p/w500/poster.jpg",
			BackdropPath: "https://image.tmdb.org/t/p/w780/backdrop.jpg",
			Overview:     "Overview",
			Genres:       []string{"Comedy"},
			Episodes: []domain.Episode{
				{Number: 1, Title: "Episode 1"},
			},
		}},
	}

	meta, found, err := service.Meta(context.Background(), "tmdb:96316:1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "TV", meta["animeType"])
	require.Equal(t, "rent-a-girlfriend", meta["slug"])
	require.Equal(t, []string{"Rent-a-Girlfriend", "彼女、お借りします"}, meta["aliases"])
	require.Equal(t, "2020-", meta["releaseInfo"])
	require.Equal(t, "2020-", meta["year"])
	require.Equal(t, "current", meta["status"])
	require.Equal(t, "24 min", meta["runtime"])
	require.Equal(t, 8.3, meta["rating"])
}

func TestMetaFallsBackToTMDBDetailsAndPersists(t *testing.T) {
	repo := &fakeCatalogRepo{anime: domain.Anime{
		TMDBID:       273467,
		SeasonNumber: 1,
		Title:        "The Warrior Princess and the Barbaric King",
	}}
	service := &stremio.Service{
		Repo: repo,
		TMDB: &fakeTMDBClient{
			details: ports.TMDBSeasonDetails{
				Title:          "The Warrior Princess and the Barbaric King",
				OriginalTitle:  "Himekishi wa Barbaroi no Yome",
				FirstAirDate:   "2026-04-09",
				Status:         "Returning Series",
				InProduction:   true,
				HasNextEpisode: true,
				TVType:         "Scripted",
				EpisodeRunTime: []int{24},
				Rating:         6.0,
			},
		},
	}

	meta, found, err := service.Meta(context.Background(), "tmdb:273467:1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "current", meta["status"])
	require.Equal(t, "24 min", meta["runtime"])
	require.Equal(t, "2026-", meta["releaseInfo"])
	require.Equal(t, "2026-", meta["year"])
	require.Equal(t, "TV", meta["animeType"])
	require.Equal(t, []string{"The Warrior Princess and the Barbaric King", "Himekishi wa Barbaroi no Yome"}, meta["aliases"])
	require.Equal(t, 6.0, meta["rating"])
	require.True(t, repo.upsertCalled)
	require.Equal(t, "current", repo.upsertAnime.Status)
}

var _ ports.CatalogRepository = (*fakeCatalogRepo)(nil)
var _ ports.TMDBClient = (*fakeTMDBClient)(nil)
