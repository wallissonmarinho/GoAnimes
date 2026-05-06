package stremio

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	CatalogIDMain   = "goanimes"
	CatalogIDWeek   = "goanimes-week"
	StremioType     = "anime"
	ManifestID      = "org.goanimes"
	ManifestName    = "GoAnimes"
	ManifestVersion = "2.0.0"
)

type Service struct {
	Repo ports.CatalogRepository
	TMDB ports.TMDBClient
}

var tracer = otel.Tracer("goanimes/stremio")

var streamQualityBlockRe = regexp.MustCompile(`(?i)\[([^\]]*\b(?:480p|720p|1080p|2160p)\b[^\]]*)\]`)

func (s *Service) Manifest(ctx context.Context) (map[string]any, error) {
	ctx, span := tracer.Start(ctx, "stremio.manifest")
	defer span.End()
	genres, err := s.Repo.ListGenres(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		genres = []string{}
	}
	genreExtra := []map[string]any{{
		"name":       "genre",
		"isRequired": false,
		"options":    genres,
	}}
	return map[string]any{
		"id":          ManifestID,
		"version":     ManifestVersion,
		"name":        ManifestName,
		"description": "RSS anime torrents with curated mapping.",
		"types":       []string{StremioType},
		"genres":      genres,
		"catalogs": []map[string]any{
			{"type": StremioType, "id": CatalogIDMain, "name": "GoAnimes", "extra": genreExtra},
			{"type": StremioType, "id": CatalogIDWeek, "name": "GoAnimes · Últimos 7 dias", "extra": genreExtra},
		},
		"resources":  []any{"catalog", "meta", "stream"},
		"idPrefixes": []string{domain.StremioIDPrefix},
	}, nil
}

func (s *Service) Catalog(ctx context.Context, catalogID string, extras map[string]string, limit, skip int) ([]map[string]any, error) {
	ctx, span := tracer.Start(ctx, "stremio.catalog")
	span.SetAttributes(
		attribute.String("catalog.id", catalogID),
		attribute.Int("catalog.limit", limit),
		attribute.Int("catalog.skip", skip),
	)
	defer span.End()
	var animes []domain.Anime
	var err error
	if catalogID == CatalogIDWeek {
		animes, err = s.Repo.ListRecent(ctx, 7, limit, skip)
	} else if g := strings.TrimSpace(extras["genre"]); g != "" {
		animes, err = s.Repo.ListByGenre(ctx, g, limit, skip)
	} else {
		animes, err = s.Repo.ListAll(ctx, limit, skip)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	metas := make([]map[string]any, 0, len(animes))
	for _, anime := range animes {
		metas = append(metas, map[string]any{
			"id":          domain.SeriesStremioID(anime.TMDBID, anime.SeasonNumber),
			"type":        StremioType,
			"name":        anime.Title,
			"poster":      anime.PosterPath,
			"genres":      anime.Genres,
			"description": anime.Overview,
		})
	}
	return metas, nil
}

func (s *Service) Meta(ctx context.Context, id string) (map[string]any, bool, error) {
	ctx, span := tracer.Start(ctx, "stremio.meta")
	span.SetAttributes(attribute.String("meta.id", id))
	defer span.End()
	tmdbID, season, ok := domain.ParseSeriesID(id)
	if !ok {
		return nil, false, nil
	}
	anime, found, err := s.Repo.GetByTMDBSeason(ctx, tmdbID, season)
	if err != nil || !found {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return nil, false, err
	}
	if needsMetaDetails(anime) && s.TMDB != nil {
		if details, detErr := s.TMDB.GetSeasonDetails(ctx, tmdbID, season); detErr == nil {
			if strings.TrimSpace(anime.Title) == "" {
				anime.Title = details.Title
			}
			if strings.TrimSpace(anime.Overview) == "" {
				anime.Overview = details.Overview
			}
			if strings.TrimSpace(anime.PosterPath) == "" {
				anime.PosterPath = details.PosterPath
			}
			if strings.TrimSpace(anime.BackdropPath) == "" {
				anime.BackdropPath = details.BackdropPath
			}
			if len(anime.Genres) == 0 && len(details.Genres) > 0 {
				anime.Genres = details.Genres
			}
		} else {
			span.RecordError(detErr)
		}
	}
	videos := make([]map[string]any, 0, len(anime.Episodes))
	for _, ep := range anime.Episodes {
		video := map[string]any{
			"id":       domain.EpisodeStremioID(anime.TMDBID, anime.SeasonNumber, ep.Number),
			"released": ep.AddedAt.Format(time.RFC3339),
			"season":   anime.SeasonNumber,
			"episode":  ep.Number,
		}
		// Use episode title if available, otherwise generate default
		if strings.TrimSpace(ep.Title) != "" {
			video["title"] = ep.Title
		} else {
			video["title"] = episodeTitle(ep.Number)
		}
		// Add episode image if available
		if strings.TrimSpace(ep.StillPath) != "" {
			video["thumbnail"] = ep.StillPath
		}
		// Add episode description if available
		if strings.TrimSpace(ep.Overview) != "" {
			video["overview"] = ep.Overview
		}
		videos = append(videos, video)
	}
	meta := map[string]any{
		"id":     domain.SeriesStremioID(anime.TMDBID, anime.SeasonNumber),
		"type":   StremioType,
		"name":   anime.Title,
		"poster": anime.PosterPath,
		"genres": anime.Genres,
		"description": func() string {
			if strings.TrimSpace(anime.Overview) != "" {
				return anime.Overview
			}
			return "Torrent releases with curated mapping."
		}(),
		"background": anime.BackdropPath,
		"videos":     videos,
	}
	return meta, true, nil
}

func needsMetaDetails(anime domain.Anime) bool {
	return strings.TrimSpace(anime.Overview) == "" || strings.TrimSpace(anime.BackdropPath) == "" || strings.TrimSpace(anime.PosterPath) == "" || strings.TrimSpace(anime.Title) == "" || len(anime.Genres) == 0
}

func (s *Service) Streams(ctx context.Context, id string) ([]map[string]any, error) {
	ctx, span := tracer.Start(ctx, "stremio.streams")
	span.SetAttributes(attribute.String("stream.id", id))
	defer span.End()
	tmdbID, season, episode, ok := domain.ParseEpisodeID(id)
	if !ok {
		return []map[string]any{}, nil
	}
	anime, found, err := s.Repo.GetByTMDBSeason(ctx, tmdbID, season)
	if err != nil || !found {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return []map[string]any{}, err
	}
	for _, ep := range anime.Episodes {
		if ep.Number != episode {
			continue
		}
		streams := make([]map[string]any, 0, len(ep.Sources))
		for _, src := range ep.Sources {
			magnet, err := resolvePlaybackURL(ctx, src.MagnetLink)
			if err != nil || strings.TrimSpace(magnet) == "" {
				continue
			}
			infoHash := magnetInfoHash(magnet)
			if strings.TrimSpace(infoHash) == "" {
				continue
			}
			streams = append(streams, map[string]any{
				"behaviorHints": map[string]any{
					"bingeGroup": id,
				},
				"fileIdx":  0,
				"infoHash": infoHash,
				"name":     buildStreamName(src.Quality, magnet),
				"title":    streamTitle(episode, magnet),
			})
		}
		return streams, nil
	}
	return []map[string]any{}, nil
}

func parseCatalogExtras(catalogPath string) (string, map[string]string, bool) {
	p := strings.TrimPrefix(strings.TrimSpace(catalogPath), "/")
	if p == "" {
		return "", nil, false
	}
	p = strings.TrimSuffix(p, ".json")
	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, false
	}
	extras := make(map[string]string)
	for _, seg := range parts[1:] {
		k, v, has := strings.Cut(seg, "=")
		if !has {
			continue
		}
		key, errK := url.PathUnescape(strings.TrimSpace(k))
		val, errV := url.PathUnescape(strings.TrimSpace(v))
		if errK != nil || errV != nil || key == "" {
			continue
		}
		extras[key] = val
	}
	return parts[0], extras, true
}

func episodeTitle(ep int) string {
	return "Episódio " + itoa(ep)
}

func buildStreamName(quality string, magnet string) string {
	name := "Torrent"
	if resolution := extractResolution(quality, magnet); strings.TrimSpace(resolution) != "" {
		name = name + " · " + strings.TrimSpace(resolution)
	}
	return name
}

func extractResolution(quality string, magnet string) string {
	fullQuality := ""
	if q := normalizedStreamQuality(quality); q != "" {
		fullQuality = q
	} else if q := qualityFromLink(magnet); q != "" {
		fullQuality = q
	}
	if fullQuality == "" {
		return ""
	}
	// Extract just the resolution (first word usually contains it)
	parts := strings.Fields(fullQuality)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func streamTitle(ep int, rawLink string) string {
	name := releaseNameFromLink(rawLink)
	if strings.TrimSpace(name) == "" {
		return episodeTitle(ep)
	}
	return episodeTitle(ep) + " · [Torrent] " + name
}

func normalizedStreamQuality(quality string) string {
	quality = strings.TrimSpace(quality)
	if quality == "" {
		return ""
	}
	return quality
}

func releaseNameFromLink(rawLink string) string {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return ""
	}
	decoded := rawLink
	if parsed, err := url.Parse(rawLink); err == nil {
		if strings.EqualFold(parsed.Scheme, "magnet") {
			if dn := strings.TrimSpace(parsed.Query().Get("dn")); dn != "" {
				if unescaped, err := url.QueryUnescape(dn); err == nil {
					decoded = unescaped
				} else {
					decoded = dn
				}
			}
		} else if parsed.Path != "" {
			if unescaped, err := url.PathUnescape(parsed.Path); err == nil {
				decoded = unescaped
			} else {
				decoded = parsed.Path
			}
		}
	}
	return decoded
}

func qualityFromLink(rawLink string) string {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return ""
	}
	decoded := rawLink
	if parsed, err := url.Parse(rawLink); err == nil {
		if strings.EqualFold(parsed.Scheme, "magnet") {
			if dn := strings.TrimSpace(parsed.Query().Get("dn")); dn != "" {
				if unescaped, err := url.QueryUnescape(dn); err == nil {
					decoded = unescaped
				} else {
					decoded = dn
				}
			}
		} else if parsed.Path != "" {
			if unescaped, err := url.PathUnescape(parsed.Path); err == nil {
				decoded = unescaped
			} else {
				decoded = parsed.Path
			}
		}
	}
	if match := streamQualityBlockRe.FindStringSubmatch(decoded); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func magnetInfoHash(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "magnet") {
		return ""
	}
	xt := strings.TrimSpace(parsed.Query().Get("xt"))
	if xt == "" {
		return ""
	}
	const prefix = "urn:btih:"
	if !strings.HasPrefix(strings.ToLower(xt), prefix) {
		return ""
	}
	h := strings.TrimSpace(xt[len(prefix):])
	if h == "" {
		return ""
	}
	return strings.ToLower(h)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func ParseCatalogPath(path string) (string, map[string]string, bool) {
	return parseCatalogExtras(path)
}
