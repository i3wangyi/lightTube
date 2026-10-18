package yt

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractVideoID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"yyUHQIec83I", "yyUHQIec83I", false},
		{"https://www.youtube.com/watch?v=yyUHQIec83I", "yyUHQIec83I", false},
		{"http://www.youtube.com/watch?v=yyUHQIec83I&t=120", "yyUHQIec83I", false},
		{"youtube.com/watch?v=yyUHQIec83I", "yyUHQIec83I", false},
		{"https://youtu.be/yyUHQIec83I", "yyUHQIec83I", false},
		{"youtu.be/yyUHQIec83I?t=42", "yyUHQIec83I", false},
		{"https://www.youtube.com/shorts/yyUHQIec83I", "yyUHQIec83I", false},
		{"https://www.youtube.com/live/yyUHQIec83I", "yyUHQIec83I", false},
		{"https://www.youtube.com/embed/yyUHQIec83I", "yyUHQIec83I", false},
		{"https://m.youtube.com/watch?v=yyUHQIec83I", "yyUHQIec83I", false},
		{"", "", true},
		{"https://example.com/video", "", true},
		{"invalid-id-too-long-123456", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ExtractVideoID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractVideoID(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractVideoID(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("ExtractVideoID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseTakeoutCSV(t *testing.T) {
	data, err := os.ReadFile("../../testdata/subscriptions.csv")
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	subs, err := ParseTakeoutCSV(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("ParseTakeoutCSV failed: %v", err)
	}

	if len(subs) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(subs))
	}

	if subs[0].ChannelID != "UC8butISFwT-Wl7EV0hUK0BQ" || subs[0].Title != "freeCodeCamp.org" {
		t.Errorf("unexpected first subscription: %+v", subs[0])
	}
	if subs[1].ChannelID != "UCdngmbVKX1Tgre699-XLlUA" || subs[1].Title != "TechWorld with Nana" {
		t.Errorf("unexpected second subscription: %+v", subs[1])
	}
}

func TestParseSearchJSON(t *testing.T) {
	data, err := os.ReadFile("../../testdata/search.json")
	if err != nil {
		t.Fatalf("failed to read search.json fixture: %v", err)
	}

	videos, err := ParseSearchJSON(data)
	if err != nil {
		t.Fatalf("ParseSearchJSON failed: %v", err)
	}

	if len(videos) != 3 {
		t.Fatalf("expected 3 videos, got %d", len(videos))
	}

	first := videos[0]
	if first.ID != "yyUHQIec83I" {
		t.Errorf("expected video ID yyUHQIec83I, got %q", first.ID)
	}
	if first.Title != "Golang Tutorial for Beginners | Full Go Course" {
		t.Errorf("unexpected title: %q", first.Title)
	}
	if first.Channel != "TechWorld with Nana" {
		t.Errorf("unexpected channel: %q", first.Channel)
	}
	if first.Duration != 12299 {
		t.Errorf("expected duration 12299, got %d", first.Duration)
	}
	if first.ViewCount <= 0 {
		t.Errorf("expected positive view count, got %d", first.ViewCount)
	}
}

func TestParseFeedXML(t *testing.T) {
	data, err := os.ReadFile("../../testdata/feed.xml")
	if err != nil {
		t.Fatalf("failed to read feed.xml fixture: %v", err)
	}

	videos, err := ParseFeedXML(data)
	if err != nil {
		t.Fatalf("ParseFeedXML failed: %v", err)
	}

	if len(videos) == 0 {
		t.Fatal("expected at least 1 video from feed fixture")
	}

	first := videos[0]
	if first.ID == "" {
		t.Errorf("expected non-empty video ID")
	}
	if first.Title == "" {
		t.Errorf("expected non-empty title")
	}
	if first.Channel != "freeCodeCamp.org" {
		t.Errorf("expected channel freeCodeCamp.org, got %q", first.Channel)
	}
	if first.PublishedAt.IsZero() {
		t.Errorf("expected non-zero published time")
	}
}

func TestParseFeedXMLHidesShorts(t *testing.T) {
	sample := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <title>Test Channel</title>
  <entry>
    <yt:videoId>normal12345</yt:videoId>
    <title>Normal Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=normal12345"/>
    <published>2026-10-17T12:00:00+00:00</published>
  </entry>
  <entry>
    <yt:videoId>shorts12345</yt:videoId>
    <title>Short Video</title>
    <link rel="alternate" href="https://www.youtube.com/shorts/shorts12345"/>
    <published>2026-10-17T13:00:00+00:00</published>
  </entry>
</feed>`

	videos, err := ParseFeedXML([]byte(sample))
	if err != nil {
		t.Fatalf("ParseFeedXML failed: %v", err)
	}

	if len(videos) != 1 {
		t.Fatalf("expected 1 video after hiding shorts, got %d", len(videos))
	}
	if videos[0].ID != "normal12345" {
		t.Errorf("expected normal video ID, got %q", videos[0].ID)
	}
}

type mockYTDLP struct {
	searchData  []byte
	searchErr   error
	resolveID   string
	resolveErr  error
	lastQuery   string
	lastResolve string
}

func (m *mockYTDLP) Search(ctx context.Context, query string) ([]byte, error) {
	m.lastQuery = query
	return m.searchData, m.searchErr
}

func (m *mockYTDLP) ResolveChannel(ctx context.Context, target string) (string, error) {
	m.lastResolve = target
	return m.resolveID, m.resolveErr
}

func TestClientSearchWithMock(t *testing.T) {
	searchFixture, err := os.ReadFile("../../testdata/search.json")
	if err != nil {
		t.Fatalf("failed to read search.json: %v", err)
	}

	mock := &mockYTDLP{searchData: searchFixture}
	client := NewClientWithOptions(mock, nil, t.TempDir())

	videos, err := client.Search(context.Background(), "golang")
	if err != nil {
		t.Fatalf("client.Search failed: %v", err)
	}

	if mock.lastQuery != "golang" {
		t.Errorf("expected query 'golang', got %q", mock.lastQuery)
	}
	if len(videos) != 3 {
		t.Errorf("expected 3 videos, got %d", len(videos))
	}
}

func TestClientResolveChannelRawUC(t *testing.T) {
	rawUC := "UC8butISFwT-Wl7EV0hUK0BQ"
	execRunner := NewExecYTDLP()
	// Raw UC ID should return immediately without executing external process.
	res, err := execRunner.ResolveChannel(context.Background(), rawUC)
	if err != nil {
		t.Fatalf("ResolveChannel failed for raw UC: %v", err)
	}
	if res != rawUC {
		t.Errorf("expected %q, got %q", rawUC, res)
	}
}

func TestFetchFeedCacheReuse(t *testing.T) {
	feedFixture, err := os.ReadFile("../../testdata/feed.xml")
	if err != nil {
		t.Fatalf("failed to read feed.xml: %v", err)
	}

	cacheDir := t.TempDir()
	channelID := "UC8butISFwT-Wl7EV0hUK0BQ"

	// Pre-populate cache with fixture and recent modification time.
	cacheFile := filepath.Join(cacheDir, channelID+".xml")
	if err := os.WriteFile(cacheFile, feedFixture, 0o644); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}

	// Create client with nil HTTP client: network requests would panic or fail.
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatal("network should not be called when cache is younger than 15 minutes")
			return nil, nil
		}),
	}

	videos, err := FetchFeed(context.Background(), client, cacheDir, channelID)
	if err != nil {
		t.Fatalf("FetchFeed failed from cache: %v", err)
	}
	if len(videos) == 0 {
		t.Fatal("expected videos from cached feed")
	}
}

func TestMergedFeedDeduplicationAndSorting(t *testing.T) {
	feedXML1 := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <title>Channel 1</title>
  <entry>
    <yt:videoId>vid-old</yt:videoId>
    <title>Old Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=vid-old"/>
    <published>2026-10-10T10:00:00Z</published>
  </entry>
  <entry>
    <yt:videoId>vid-shared</yt:videoId>
    <title>Shared Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=vid-shared"/>
    <published>2026-10-15T10:00:00Z</published>
  </entry>
</feed>`

	feedXML2 := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <title>Channel 2</title>
  <entry>
    <yt:videoId>vid-newest</yt:videoId>
    <title>Newest Video</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=vid-newest"/>
    <published>2026-10-17T10:00:00Z</published>
  </entry>
  <entry>
    <yt:videoId>vid-shared</yt:videoId>
    <title>Shared Video Duplicate</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=vid-shared"/>
    <published>2026-10-15T10:00:00Z</published>
  </entry>
</feed>`

	cacheDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(cacheDir, "ch1.xml"), []byte(feedXML1), 0o644)
	_ = os.WriteFile(filepath.Join(cacheDir, "ch2.xml"), []byte(feedXML2), 0o644)

	merged, err := FetchMergedFeed(context.Background(), nil, cacheDir, []string{"ch1", "ch2"})
	if err != nil {
		t.Fatalf("FetchMergedFeed failed: %v", err)
	}

	if len(merged) != 3 {
		t.Fatalf("expected 3 unique videos, got %d", len(merged))
	}

	if merged[0].ID != "vid-newest" {
		t.Errorf("expected first video to be vid-newest, got %q", merged[0].ID)
	}
	if merged[1].ID != "vid-shared" {
		t.Errorf("expected second video to be vid-shared, got %q", merged[1].ID)
	}
	if merged[2].ID != "vid-old" {
		t.Errorf("expected third video to be vid-old, got %q", merged[2].ID)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		sec  int
		want string
	}{
		{0, "00:00"},
		{59, "00:59"},
		{60, "01:00"},
		{214, "03:34"},
		{3600, "1:00:00"},
		{12299, "3:24:59"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.sec)
		if got != tt.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
