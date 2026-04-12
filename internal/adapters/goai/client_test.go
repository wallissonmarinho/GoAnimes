package goai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func TestClient_AuditSeries_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audit/series" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Fatalf("auth header")
		}
		_ = json.NewEncoder(w).Encode(domain.GoaiSeriesAuditResponse{TheTVDBSeriesID: 99, Confidence: 0.9})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "k", time.Second, "test/1")
	resp, err := c.AuditSeries(context.Background(), domain.GoaiSeriesAuditRequest{TorrentTitle: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.TheTVDBSeriesID != 99 {
		t.Fatalf("got %+v", resp)
	}
}

func TestClient_AuditSeries_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("no"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "k", time.Second, "test/1")
	_, err := c.AuditSeries(context.Background(), domain.GoaiSeriesAuditRequest{TorrentTitle: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}
