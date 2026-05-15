package domain

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	StremioIDPrefix = "tmdb"
)

func SeriesStremioID(tmdbID, season int) string {
	return fmt.Sprintf("%s:%d:%d", StremioIDPrefix, tmdbID, season)
}

func AggregateSeriesStremioID(tmdbID int) string {
	return fmt.Sprintf("%s:%d", StremioIDPrefix, tmdbID)
}

func EpisodeStremioID(tmdbID, season, episode int) string {
	return fmt.Sprintf("%s:%d:%d:%d", StremioIDPrefix, tmdbID, season, episode)
}

func ParseSeriesID(id string) (tmdbID int, season int, ok bool) {
	parts := strings.Split(strings.TrimSpace(id), ":")
	if len(parts) == 2 && parts[0] == StremioIDPrefix {
		tid, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
		return tid, 0, true
	}
	if len(parts) != 3 || parts[0] != StremioIDPrefix {
		return 0, 0, false
	}
	tid, err1 := strconv.Atoi(parts[1])
	sea, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return tid, sea, true
}

func ParseEpisodeID(id string) (tmdbID int, season int, episode int, ok bool) {
	parts := strings.Split(strings.TrimSpace(id), ":")
	if len(parts) != 4 || parts[0] != StremioIDPrefix {
		return 0, 0, 0, false
	}
	tid, err1 := strconv.Atoi(parts[1])
	sea, err2 := strconv.Atoi(parts[2])
	ep, err3 := strconv.Atoi(parts[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return tid, sea, ep, true
}
