package tmdbapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/ports"
)

type Client struct {
	key     string
	http    *http.Client
	baseURL string
}

var genericEpisodeTitleRe = regexp.MustCompile(`(?i)^(epis[oó]dio|episode)\s+\d+$`)

const animationGenreID = 16

func NewClient(apiKey string, timeout time.Duration) *Client {
	return &Client{
		key:     strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: timeout},
		baseURL: "https://api.themoviedb.org/3",
	}
}

func (c *Client) SearchSeries(ctx context.Context, query string) (ports.TMDBSearchResult, bool, error) {
	if c == nil || c.key == "" {
		return ports.TMDBSearchResult{}, false, errors.New("tmdb api key not configured")
	}
	q := url.Values{}
	q.Set("api_key", c.key)
	q.Set("query", strings.TrimSpace(query))
	q.Set("language", "pt-BR")
	endpoint := c.baseURL + "/search/tv?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ports.TMDBSearchResult{}, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ports.TMDBSearchResult{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ports.TMDBSearchResult{}, false, fmt.Errorf("tmdb search failed: %s", resp.Status)
	}
	var payload struct {
		Results []struct {
			ID               int    `json:"id"`
			Name             string `json:"name"`
			OriginalName     string `json:"original_name"`
			OriginalLanguage string `json:"original_language"`
			FirstAirDate     string `json:"first_air_date"`
			GenreIDs         []int  `json:"genre_ids"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ports.TMDBSearchResult{}, false, err
	}
	if len(payload.Results) == 0 {
		return ports.TMDBSearchResult{}, false, nil
	}
	candidates := make([]tmdbSearchCandidate, 0, len(payload.Results))
	for _, res := range payload.Results {
		candidates = append(candidates, tmdbSearchCandidate{
			ID:               res.ID,
			Name:             strings.TrimSpace(res.Name),
			OriginalName:     strings.TrimSpace(res.OriginalName),
			OriginalLanguage: strings.TrimSpace(res.OriginalLanguage),
			FirstAirDate:     strings.TrimSpace(res.FirstAirDate),
			GenreIDs:         append([]int(nil), res.GenreIDs...),
		})
	}
	best, ok := chooseBestSeriesCandidate(strings.TrimSpace(query), candidates)
	if !ok {
		return ports.TMDBSearchResult{}, false, nil
	}
	return ports.TMDBSearchResult{TMDBID: best.ID, Title: best.Name}, true, nil
}

type tmdbSearchCandidate struct {
	ID               int
	Name             string
	OriginalName     string
	OriginalLanguage string
	FirstAirDate     string
	GenreIDs         []int
}

func chooseBestSeriesCandidate(query string, candidates []tmdbSearchCandidate) (tmdbSearchCandidate, bool) {
	if len(candidates) == 0 {
		return tmdbSearchCandidate{}, false
	}
	queryNorm := normalizeSearchText(query)
	queryTokens := tokenSet(queryNorm)
	if len(queryTokens) == 0 {
		return tmdbSearchCandidate{}, false
	}

	scored := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score := candidateSearchScore(queryNorm, queryTokens, candidate)
		if score < 0 {
			continue
		}
		scored = append(scored, scoredCandidate{candidate: candidate, score: score})
	}
	if len(scored) == 0 {
		return tmdbSearchCandidate{}, false
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].candidate.ID < scored[j].candidate.ID
	})

	best := scored[0]
	if !hasAnimationGenre(best.candidate.GenreIDs) {
		return tmdbSearchCandidate{}, false
	}
	if best.score < 4 {
		return tmdbSearchCandidate{}, false
	}
	return best.candidate, true
}

type scoredCandidate struct {
	candidate tmdbSearchCandidate
	score     int
}

func candidateSearchScore(queryNorm string, queryTokens map[string]struct{}, candidate tmdbSearchCandidate) int {
	nameNorm := normalizeSearchText(candidate.Name)
	originalNorm := normalizeSearchText(candidate.OriginalName)

	bestTitleScore := similarityScore(queryNorm, queryTokens, nameNorm)
	if score := similarityScore(queryNorm, queryTokens, originalNorm); score > bestTitleScore {
		bestTitleScore = score
	}
	if bestTitleScore <= 0 {
		return -1
	}

	score := bestTitleScore
	if hasAnimationGenre(candidate.GenreIDs) {
		score += 10
	}
	switch strings.ToLower(candidate.OriginalLanguage) {
	case "ja":
		score += 3
	case "zh", "ko":
		score += 2
	}
	if candidate.FirstAirDate >= "2015-01-01" {
		score += 1
	}
	return score
}

func similarityScore(queryNorm string, queryTokens map[string]struct{}, candidateNorm string) int {
	if candidateNorm == "" {
		return 0
	}
	if candidateNorm == queryNorm {
		return 8
	}
	candidateTokens := tokenSet(candidateNorm)
	if len(candidateTokens) == 0 {
		return 0
	}
	overlap := 0
	for token := range queryTokens {
		if _, ok := candidateTokens[token]; ok {
			overlap++
		}
	}
	if overlap == 0 {
		return 0
	}
	if overlap == len(queryTokens) {
		if strings.Contains(candidateNorm, queryNorm) || strings.Contains(queryNorm, candidateNorm) {
			return 7
		}
		return 6
	}
	if overlap >= 2 {
		return 4 + overlap
	}
	return 2
}

func hasAnimationGenre(ids []int) bool {
	for _, id := range ids {
		if id == animationGenreID {
			return true
		}
	}
	return false
}

func normalizeSearchText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(
		":", " ",
		"-", " ",
		"_", " ",
		"'", "",
		".", " ",
		",", " ",
		"!", " ",
		"?", " ",
		"(", " ",
		")", " ",
	)
	s = replacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func tokenSet(s string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, token := range strings.Fields(s) {
		out[token] = struct{}{}
	}
	return out
}

func (c *Client) GetSeasonDetails(ctx context.Context, tmdbID, season int) (ports.TMDBSeasonDetails, error) {
	if c == nil || c.key == "" {
		return ports.TMDBSeasonDetails{}, errors.New("tmdb api key not configured")
	}
	showPayload, err := c.getShowDetails(ctx, tmdbID, "pt-BR")
	if err != nil {
		return ports.TMDBSeasonDetails{}, err
	}
	seasonPayload, err := c.getSeasonDetails(ctx, tmdbID, season, "pt-BR")
	if err != nil {
		return ports.TMDBSeasonDetails{}, err
	}

	genres := make([]string, 0, len(showPayload.Genres))
	for _, g := range showPayload.Genres {
		if g.Name != "" {
			genres = append(genres, g.Name)
		}
	}

	poster := strings.TrimSpace(seasonPayload.Poster)
	if poster == "" {
		poster = strings.TrimSpace(showPayload.Poster)
	}
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = "https://image.tmdb.org/t/p/w500" + poster
	}

	title := strings.TrimSpace(showPayload.Name)
	if title == "" {
		title = strings.TrimSpace(showPayload.OriginalName)
	}

	rating := showPayload.Rating
	if rating == 0 && seasonPayload.VoteAverage > 0 {
		rating = seasonPayload.VoteAverage
	}

	return ports.TMDBSeasonDetails{
		Title:             title,
		OriginalTitle:     strings.TrimSpace(showPayload.OriginalName),
		Overview:          strings.TrimSpace(showPayload.Overview),
		PosterPath:        poster,
		BackdropPath:      imageURL(showPayload.Backdrop),
		LogoPath:          "",
		Genres:            genres,
		Rating:            rating,
		VoteCount:         showPayload.VoteCount,
		Popularity:        showPayload.Popularity,
		FirstAirDate:      strings.TrimSpace(showPayload.FirstAirDate),
		LastAirDate:       strings.TrimSpace(showPayload.LastAirDate),
		LastEpisodeAirDate: episodeAirDate(showPayload.LastEpisodeToAir),
		LastEpisodeNumber: episodeNumber(showPayload.LastEpisodeToAir),
		NextEpisodeAirDate: episodeAirDate(showPayload.NextEpisodeToAir),
		NextEpisodeNumber: episodeNumber(showPayload.NextEpisodeToAir),
		Status:            strings.TrimSpace(showPayload.Status),
		InProduction:      showPayload.InProduction,
		HasNextEpisode:    showPayload.NextEpisodeToAir != nil,
		TVType:            strings.TrimSpace(showPayload.TVType),
		EpisodeRunTime:    showPayload.EpisodeRunTime,
		SeasonRunTime:     extractEpisodeRuntimes(seasonPayload.Episodes),
		SeasonVoteAverage: seasonPayload.VoteAverage,
	}, nil
}

func imageURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http") {
		return path
	}
	return "https://image.tmdb.org/t/p/w780" + path
}

func episodeNumber(ep *tmdbAiredEpisode) int {
	if ep == nil || ep.EpisodeNumber <= 0 {
		return 0
	}
	return ep.EpisodeNumber
}

func (c *Client) GetEpisodeDetails(ctx context.Context, tmdbID, season, episode int) (ports.TMDBEpisodeDetails, error) {
	if c == nil || c.key == "" {
		return ports.TMDBEpisodeDetails{}, errors.New("tmdb api key not configured")
	}
	localized, err := c.getEpisodeDetails(ctx, tmdbID, season, episode, "pt-BR")
	if err != nil {
		return ports.TMDBEpisodeDetails{}, err
	}
	title := strings.TrimSpace(localized.Name)
	overview := strings.TrimSpace(localized.Overview)
	still := strings.TrimSpace(localized.Still)

	if title == "" || isGenericEpisodeTitle(title) {
		original, fallbackErr := c.getEpisodeDetails(ctx, tmdbID, season, episode, "")
		if fallbackErr == nil {
			origTitle := strings.TrimSpace(original.Name)
			if origTitle != "" && !isGenericEpisodeTitle(origTitle) {
				title = origTitle
			}
			if overview == "" {
				overview = strings.TrimSpace(original.Overview)
			}
			if still == "" {
				still = strings.TrimSpace(original.Still)
			}
		}
	}

	return ports.TMDBEpisodeDetails{
		AirDate:   strings.TrimSpace(localized.AirDate),
		Title:     title,
		Overview:  overview,
		StillPath: imageURL(still),
	}, nil
}

type tmdbGenre struct {
	Name string `json:"name"`
}

type tmdbEpisodePayload struct {
	Name     string `json:"name"`
	Overview string `json:"overview"`
	Still    string `json:"still_path"`
	AirDate  string `json:"air_date"`
	Runtime  *int   `json:"runtime"`
}

type tmdbShowPayload struct {
	Name             string        `json:"name"`
	OriginalName     string        `json:"original_name"`
	Overview         string        `json:"overview"`
	Poster           string        `json:"poster_path"`
	Backdrop         string        `json:"backdrop_path"`
	Genres           []tmdbGenre   `json:"genres"`
	Rating           float64       `json:"vote_average"`
	VoteCount        int           `json:"vote_count"`
	Popularity       float64       `json:"popularity"`
	FirstAirDate     string        `json:"first_air_date"`
	LastAirDate      string        `json:"last_air_date"`
	Status           string        `json:"status"`
	InProduction     bool          `json:"in_production"`
	LastEpisodeToAir *tmdbAiredEpisode `json:"last_episode_to_air"`
	NextEpisodeToAir *tmdbAiredEpisode `json:"next_episode_to_air"`
	EpisodeRunTime   []int         `json:"episode_run_time"`
	TVType           string        `json:"type"`
}

type tmdbAiredEpisode struct {
	AirDate       string `json:"air_date"`
	EpisodeNumber int    `json:"episode_number"`
}

type tmdbSeasonPayload struct {
	Name        string               `json:"name"`
	Overview    string               `json:"overview"`
	Poster      string               `json:"poster_path"`
	VoteAverage float64              `json:"vote_average"`
	Episodes    []tmdbEpisodePayload `json:"episodes"`
}

func (c *Client) getShowDetails(ctx context.Context, tmdbID int, language string) (tmdbShowPayload, error) {
	var payload tmdbShowPayload
	path := fmt.Sprintf("/tv/%d", tmdbID)
	if err := c.get(ctx, path, language, &payload); err != nil {
		return tmdbShowPayload{}, fmt.Errorf("tmdb details failed: %w", err)
	}
	return payload, nil
}

func (c *Client) getSeasonDetails(ctx context.Context, tmdbID, season int, language string) (tmdbSeasonPayload, error) {
	var payload tmdbSeasonPayload
	path := fmt.Sprintf("/tv/%d/season/%d", tmdbID, season)
	if err := c.get(ctx, path, language, &payload); err != nil {
		return tmdbSeasonPayload{}, fmt.Errorf("tmdb season details failed: %w", err)
	}
	return payload, nil
}

func (c *Client) getEpisodeDetails(ctx context.Context, tmdbID, season, episode int, language string) (tmdbEpisodePayload, error) {
	var payload tmdbEpisodePayload
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", tmdbID, season, episode)
	if err := c.get(ctx, path, language, &payload); err != nil {
		return tmdbEpisodePayload{}, fmt.Errorf("tmdb episode details failed: %w", err)
	}
	return payload, nil
}

func (c *Client) get(ctx context.Context, path, language string, out any) error {
	q := url.Values{}
	q.Set("api_key", c.key)
	if strings.TrimSpace(language) != "" {
		q.Set("language", strings.TrimSpace(language))
	}
	endpoint := fmt.Sprintf("%s%s?%s", c.baseURL, path, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func extractEpisodeRuntimes(episodes []tmdbEpisodePayload) []int {
	runtimes := make([]int, 0, len(episodes))
	for _, episode := range episodes {
		if episode.Runtime == nil || *episode.Runtime <= 0 {
			continue
		}
		runtimes = append(runtimes, *episode.Runtime)
	}
	return runtimes
}

func episodeAirDate(ep *tmdbAiredEpisode) string {
	if ep == nil {
		return ""
	}
	return strings.TrimSpace(ep.AirDate)
}

func isGenericEpisodeTitle(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	return genericEpisodeTitleRe.MatchString(name)
}
