package rss

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
)

func TestFetchKeepsOnlyPortugueseSubtitleItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example</title>
    <link>https://example.test</link>
    <description>Example feed</description>
    <item>
      <title>[Torrent] Liar Game - 05 (HEVC) [1080p CR WEBRip HEVC AAC][Encoded]</title>
      <link>https://example.test/1</link>
      <pubDate>Mon, 04 May 2026 19:34:18 +0000</pubDate>
      <description><![CDATA[Subtitles: [us][br][mx][es] | Size: 470.05MB]]></description>
    </item>
    <item>
      <title>[ToonsHub] LIAR GAME S01E05 1080p CR WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=ja;S=en,ptbr,frfr,de;}</title>
      <link>https://example.test/2</link>
      <pubDate>Mon, 04 May 2026 17:31:50 +0000</pubDate>
      <description>[ToonsHub] LIAR GAME S01E05 1080p CR WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=ja;S=en,ptbr,frfr,de;}</description>
    </item>
    <item>
      <title>[Other] Liar Game - 06 1080p WEBRip AAC</title>
      <link>https://example.test/3</link>
      <pubDate>Mon, 04 May 2026 20:10:00 +0000</pubDate>
      <description>Subtitles: [us][mx][es]</description>
    </item>
  </channel>
</rss>`)
	}))
	defer server.Close()

	reader := NewReader()
	items, err := reader.Fetch(context.Background(), domain.Feed{
		Name: "Erai",
		URL:  server.URL,
		Type: domain.FeedTypeRSS,
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items with Portuguese subtitles, got %d", len(items))
	}
	if items[0].Title != "[Torrent] Liar Game - 05 (HEVC) [1080p CR WEBRip HEVC AAC][Encoded]" {
		t.Fatalf("unexpected first item title: %q", items[0].Title)
	}
	if items[1].Title != "[ToonsHub] LIAR GAME S01E05 1080p CR WEB-DL AAC2.0 H.264 (Multi-Subs) {Tags:L0;V9;C1;A=ja;S=en,ptbr,frfr,de;}" {
		t.Fatalf("unexpected second item title: %q", items[1].Title)
	}
}
