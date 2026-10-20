package yt

import (
	"testing"
)

func TestParsePodcastsFeedJSON(t *testing.T) {
	// 1. Valid response with multiple shows
	validJSON := []byte(`{
		"id": "podcasts",
		"title": "podcasts",
		"entries": [
			{
				"id": "PLshowHistory0000000001",
				"title": "The Long View Podcast",
				"channel": "Perspective Studio",
				"url": "https://www.youtube.com/playlist?list=PLshowHistory0000000001",
				"thumbnails": [
					{"url": "https://i.ytimg.com/vi/thumb1_small.jpg", "height": 100},
					{"url": "https://i.ytimg.com/vi/thumb1_large.jpg", "height": 600}
				]
			},
			{
				"id": "PLshowTech000000000002",
				"title": "Systems of Living",
				"uploader": "Common Ground",
				"url": "https://www.youtube.com/playlist?list=PLshowTech000000000002",
				"thumbnails": [
					{"url": "https://i.ytimg.com/vi/thumb2.jpg", "height": 400}
				]
			}
		]
	}`)

	shows, err := ParsePodcastsFeedJSON(validJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing valid podcasts feed: %v", err)
	}
	if len(shows) != 2 {
		t.Fatalf("expected 2 shows, got %d", len(shows))
	}
	if shows[0].ID != "PLshowHistory0000000001" || shows[0].Title != "The Long View Podcast" {
		t.Errorf("show 0 mismatch: %+v", shows[0])
	}
	if shows[0].ThumbnailURL != "https://i.ytimg.com/vi/thumb1_large.jpg" {
		t.Errorf("expected highest resolution thumbnail, got %q", shows[0].ThumbnailURL)
	}

	// 2. Empty response
	emptyJSON := []byte(`{"id": "podcasts", "entries": []}`)
	sEmpty, err := ParsePodcastsFeedJSON(emptyJSON)
	if err != nil {
		t.Fatalf("unexpected error on empty: %v", err)
	}
	if len(sEmpty) != 0 {
		t.Errorf("expected 0 shows on empty entries, got %d", len(sEmpty))
	}

	// 3. Null / whitespace response
	sNull, err := ParsePodcastsFeedJSON([]byte("   null   "))
	if err != nil {
		t.Fatalf("unexpected error on null: %v", err)
	}
	if len(sNull) != 0 {
		t.Errorf("expected 0 shows on null, got %d", len(sNull))
	}

	// 4. Invalid / corrupt JSON
	_, err = ParsePodcastsFeedJSON([]byte(`{"id": "podcasts", "entries": [ { invalid `))
	if err == nil {
		t.Errorf("expected error on corrupt JSON, got nil")
	}

	// 5. Changed schema response
	changedJSON := []byte(`{
		"new_schema_version": 4,
		"entries": [
			{
				"id": "PLnewShow0000000003",
				"title": "Unlisted Podcast Show",
				"channel": "View full playlist",
				"unexpected_blob": {"x": 1}
			},
			{
				"no_id_or_title": true
			}
		]
	}`)
	sChanged, err := ParsePodcastsFeedJSON(changedJSON)
	if err != nil {
		t.Fatalf("unexpected error on changed schema: %v", err)
	}
	if len(sChanged) != 1 {
		t.Fatalf("expected 1 valid show, got %d", len(sChanged))
	}
	if sChanged[0].ID != "PLnewShow0000000003" {
		t.Errorf("expected ID 'PLnewShow0000000003', got %q", sChanged[0].ID)
	}
	if sChanged[0].Channel != "" {
		t.Errorf("expected 'View full playlist' channel label to be dropped, got %q", sChanged[0].Channel)
	}
}

func TestParsePodcastPlaylistSearchKeepsPlaylistIdentity(t *testing.T) {
	data := []byte(`{"entries":[{"id":"PL_fixture","title":"Fixture show episodes","url":"https://www.youtube.com/playlist?list=PL_fixture"},{"id":"video_fixture1","title":"ordinary video","url":"https://www.youtube.com/watch?v=video_fixture1"}]}`)
	got, err := ParsePodcastPlaylistSearch(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "PL_fixture" || got[0].URL != "https://www.youtube.com/playlist?list=PL_fixture" {
		t.Fatalf("playlist search lost source identity: %+v", got)
	}
}
