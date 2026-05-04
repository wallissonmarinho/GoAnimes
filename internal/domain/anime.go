package domain

import (
	"errors"
	"strings"
	"time"
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
	Genres        []string
	Rating        float64
	PosterPath    string
	Episodes      []Episode
	MappingStatus MappingStatus
	UpdatedAt     time.Time
}

type Episode struct {
	Number  int
	Sources []Source
	AddedAt time.Time
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
	if src.MagnetLink == "" {
		return false
	}
	for _, existing := range e.Sources {
		if strings.EqualFold(existing.MagnetLink, src.MagnetLink) {
			return false
		}
	}
	e.Sources = append(e.Sources, src)
	return true
}
