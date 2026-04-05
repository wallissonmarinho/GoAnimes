package rsssync

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// maxEraiPerAnimeFeedFetches limits HTTP fetches to anime-list/{slug}/feed per sync (0 = unlimited).
func maxEraiPerAnimeFeedFetches() int {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_MAX_PER_ANIME_FEEDS"))
	if v == "" {
		return 200
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 200
	}
	return n
}

// eraiPerAnimeFetchDelay is the pause between successive per-anime Erai feed GETs (reduces 429).
func eraiPerAnimeFetchDelay() time.Duration {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_PER_ANIME_DELAY"))
	if v == "" {
		return 400 * time.Millisecond
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 400 * time.Millisecond
	}
	return d
}

// eraiPerAnimeFetchMaxAttempts is GET retries per slug on 429/503 (default 5 = 1 try + up to 4 backoff waits).
func eraiPerAnimeFetchMaxAttempts() int {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_PER_ANIME_MAX_ATTEMPTS"))
	if v == "" {
		return 5
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 5
	}
	if n > 20 {
		return 20
	}
	return n
}

func eraiPerAnimeRetryBaseBackoff() time.Duration {
	v := strings.TrimSpace(os.Getenv("GOANIMES_ERAI_PER_ANIME_RETRY_BACKOFF"))
	if v == "" {
		return 2 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 2 * time.Second
	}
	return d
}
