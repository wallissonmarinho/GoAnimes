package tmdbapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetEpisodeDetailsFallbacksWhenLocalizedTitleIsGeneric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/273467/season/1/episode/1" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("language") == "pt-BR" {
			_, _ = w.Write([]byte(`{"name":"Episodio 1","overview":"","still_path":""}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"The Defeated Warrior Princess, Taken Captive","overview":"Original overview","still_path":"/still-original.jpg"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", time.Second)
	client.baseURL = server.URL

	details, err := client.GetEpisodeDetails(context.Background(), 273467, 1, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if details.Title != "The Defeated Warrior Princess, Taken Captive" {
		t.Fatalf("expected fallback original episode title, got %q", details.Title)
	}
	if details.Overview != "Original overview" {
		t.Fatalf("expected overview fallback, got %q", details.Overview)
	}
	if details.StillPath == "" {
		t.Fatal("expected still path to be filled from fallback response")
	}
}

func TestGetSeasonDetailsReadsShowAndSeasonData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tv/273467":
			_, _ = w.Write([]byte(`{
				"name":"The Warrior Princess and the Barbaric King",
				"original_name":"Himekishi wa Barbaroi no Yome",
				"overview":"Show overview",
				"poster_path":"/show-poster.jpg",
				"backdrop_path":"/show-backdrop.jpg",
				"genres":[{"name":"Fantasy"}],
				"vote_average":8.1,
				"first_air_date":"2026-04-09",
				"last_air_date":"2026-06-25",
				"status":"Returning Series",
				"in_production":true,
				"next_episode_to_air":{"id":1},
				"episode_run_time":[24],
				"type":"Scripted",
				"networks":[{"logo_path":"/network-logo.jpg"}]
			}`))
		case "/tv/273467/season/1":
			_, _ = w.Write([]byte(`{
				"poster_path":"/season-poster.jpg",
				"vote_average":6.0,
				"networks":[{"logo_path":"/season-network-logo.jpg"}],
				"episodes":[{"runtime":23},{"runtime":24},{"runtime":null}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", time.Second)
	client.baseURL = server.URL

	details, err := client.GetSeasonDetails(context.Background(), 273467, 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if details.Title != "The Warrior Princess and the Barbaric King" {
		t.Fatalf("unexpected title %q", details.Title)
	}
	if details.OriginalTitle != "Himekishi wa Barbaroi no Yome" {
		t.Fatalf("unexpected original title %q", details.OriginalTitle)
	}
	if details.Status != "Returning Series" || !details.InProduction || !details.HasNextEpisode {
		t.Fatal("expected status, production and next episode data from show endpoint")
	}
	if len(details.EpisodeRunTime) == 0 || details.EpisodeRunTime[0] != 24 {
		t.Fatalf("unexpected episode runtime list: %#v", details.EpisodeRunTime)
	}
	if len(details.SeasonRunTime) != 2 || details.SeasonRunTime[0] != 23 || details.SeasonRunTime[1] != 24 {
		t.Fatalf("unexpected season runtime list: %#v", details.SeasonRunTime)
	}
	if details.LogoPath != "" {
		t.Fatalf("expected logo path to remain empty, got %q", details.LogoPath)
	}
}
