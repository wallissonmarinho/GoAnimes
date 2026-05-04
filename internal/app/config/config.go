package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	MongoURI    string
	MongoDB     string
	AdminAPIKey string
	TMDBAPIKey  string
	HTTPTimeout time.Duration
}

func Load() Config {
	return Config{
		MongoURI:    getenv("GOANIMES_MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:     getenv("GOANIMES_MONGO_DB", "goanimes"),
		AdminAPIKey: getenv("GOANIMES_ADMIN_API_KEY", getenv("ADMIN_API_KEY", "")),
		TMDBAPIKey:  getenv("GOANIMES_TMDB_API_KEY", ""),
		HTTPTimeout: durationEnv("GOANIMES_HTTP_TIMEOUT", 45*time.Second),
	}
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func durationEnv(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
