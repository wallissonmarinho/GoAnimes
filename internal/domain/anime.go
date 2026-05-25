package domain

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

var (
	eraiProviderRe       = regexp.MustCompile(`(?i)\berai(?:-raws)?\b`)
	trailingHashTagRe    = regexp.MustCompile(`\[[A-F0-9]{8}\]$`)
	repeatedWhitespaceRe = regexp.MustCompile(`\s+`)
)

type MappingStatus string

const (
	MappingStatusMapped    MappingStatus = "mapped"
	MappingStatusUnmatched MappingStatus = "unmatched"
)

type Anime struct {
	ID            string
	TMDBID        int
	SeasonNumber  int
	Title         string
	AnimeType     string
	Slug          string
	Aliases       []string
	LogoPath      string
	ReleaseInfo   string
	Year          string
	Status        string
	Runtime       string
	Overview      string
	Genres        []string
	Rating        float64
	VoteCount     int
	Popularity    float64
	PosterPath    string
	BackdropPath  string
	LastEpisodeAt string
	LastEpisodeNo int
	NextEpisodeAt string
	NextEpisodeNo int
	Episodes      []Episode
	MappingStatus MappingStatus
	UpdatedAt     time.Time
}

type Episode struct {
	Number    int
	AirDate   string
	Title     string
	Overview  string
	StillPath string
	Sources   []Source
	AddedAt   time.Time
}

type Source struct {
	Provider   string
	MagnetLink string
	Quality    string
}

func (a *Anime) Validate() error {
	if a.TMDBID <= 0 {
		return errors.New("tmdb_id is required")
	}
	if a.SeasonNumber <= 0 {
		return errors.New("season_number is required")
	}
	if strings.TrimSpace(a.Title) == "" {
		return errors.New("title is required")
	}
	return nil
}

func (a *Anime) EnsureEpisode(num int) *Episode {
	for i := range a.Episodes {
		if a.Episodes[i].Number == num {
			return &a.Episodes[i]
		}
	}
	ep := Episode{Number: num, AddedAt: time.Now().UTC()}
	a.Episodes = append(a.Episodes, ep)
	return &a.Episodes[len(a.Episodes)-1]
}

func (e *Episode) AddSource(src Source) bool {
	src.MagnetLink = strings.TrimSpace(src.MagnetLink)
	src.Provider = strings.TrimSpace(src.Provider)
	if src.MagnetLink == "" || src.Provider == "" {
		return false
	}
	newProviderKey := canonicalProviderKey(src.Provider)
	newReleaseKey := canonicalReleaseKey(src.MagnetLink)
	for _, existing := range e.Sources {
		if strings.EqualFold(existing.MagnetLink, src.MagnetLink) && strings.EqualFold(existing.Provider, src.Provider) {
			return false
		}
		existingReleaseKey := canonicalReleaseKey(existing.MagnetLink)
		if newReleaseKey != "" && existingReleaseKey != "" && newReleaseKey == existingReleaseKey {
			return false
		}
		if newProviderKey != "" && newReleaseKey != "" &&
			newProviderKey == canonicalProviderKey(existing.Provider) &&
			newReleaseKey == existingReleaseKey {
			return false
		}
	}
	e.Sources = append(e.Sources, src)
	return true
}

func canonicalProviderKey(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return ""
	}
	if eraiProviderRe.MatchString(provider) {
		return "erai"
	}
	return provider
}

func canonicalReleaseKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil {
		switch {
		case strings.EqualFold(parsed.Scheme, "magnet"):
			if dn := strings.TrimSpace(parsed.Query().Get("dn")); dn != "" {
				if decoded, decErr := url.QueryUnescape(dn); decErr == nil && strings.TrimSpace(decoded) != "" {
					raw = decoded
				} else {
					raw = dn
				}
			}
		case parsed.Scheme == "http" || parsed.Scheme == "https":
			base := strings.TrimSpace(path.Base(parsed.Path))
			if decoded, decErr := url.QueryUnescape(base); decErr == nil && strings.TrimSpace(decoded) != "" {
				raw = decoded
			} else {
				raw = base
			}
		}
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".torrent")
	raw = strings.TrimSuffix(raw, ".mkv")
	raw = trailingHashTagRe.ReplaceAllString(raw, "")
	raw = repeatedWhitespaceRe.ReplaceAllString(strings.TrimSpace(raw), " ")
	return strings.ToLower(raw)
}
