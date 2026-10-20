package yt

import (
	"testing"
)

func TestParseLikedMusicJSON(t *testing.T) {
	// 1. Valid liked music response
	validJSON := []byte(`{
		"id": "LM",
		"title": "Liked Music",
		"entries": [
			{
				"id": "trkBlueHour01",
				"title": "Blue Hour",
				"channel": "Low Window",
				"duration": 252,
				"view_count": 120000,
				"url": "https://music.youtube.com/watch?v=trkBlueHour01"
			},
			{
				"id": "trkGeometry02",
				"title": "Soft Geometry",
				"uploader": "Paper Echo",
				"duration": 228,
				"view_count": 94000,
				"url": "https://www.youtube.com/watch?v=trkGeometry02"
			}
		]
	}`)

	tracks, err := ParseLikedMusicJSON(validJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing liked music: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if tracks[0].ID != "trkBlueHour01" || tracks[0].Channel != "Low Window" || tracks[0].Duration != 252 {
		t.Errorf("track 0 mismatch: %+v", tracks[0])
	}
	if tracks[1].Channel != "Paper Echo" {
		t.Errorf("expected track 1 uploader fallback, got %q", tracks[1].Channel)
	}

	// 2. Empty response
	emptyJSON := []byte(`{"id": "LM", "entries": []}`)
	tEmpty, err := ParseLikedMusicJSON(emptyJSON)
	if err != nil {
		t.Fatalf("unexpected error on empty: %v", err)
	}
	if len(tEmpty) != 0 {
		t.Errorf("expected 0 tracks on empty entries, got %d", len(tEmpty))
	}

	// 3. Null / whitespace response
	tNull, err := ParseLikedMusicJSON([]byte("   null   "))
	if err != nil {
		t.Fatalf("unexpected error on null: %v", err)
	}
	if len(tNull) != 0 {
		t.Errorf("expected 0 tracks on null, got %d", len(tNull))
	}

	// 4. Invalid / corrupt JSON
	_, err = ParseLikedMusicJSON([]byte(`{"id": "LM", "entries": [ { invalid `))
	if err == nil {
		t.Errorf("expected error on corrupt JSON, got nil")
	}

	// 5. Changed schema response
	changedJSON := []byte(`{
		"new_format": true,
		"entries": [
			{
				"id": "trkNew0000003",
				"title": "Ambient Glow",
				"extra_field": "ignore"
			},
			{
				"missing_id": true
			}
		]
	}`)
	tChanged, err := ParseLikedMusicJSON(changedJSON)
	if err != nil {
		t.Fatalf("unexpected error on changed schema: %v", err)
	}
	if len(tChanged) != 1 {
		t.Fatalf("expected 1 valid track, got %d", len(tChanged))
	}
	if tChanged[0].ID != "trkNew0000003" {
		t.Errorf("expected ID 'trkNew0000003', got %q", tChanged[0].ID)
	}
}

func TestParseMusicAlbumJSON(t *testing.T) {
	// 1. Valid album page
	albumJSON := []byte(`{
		"id": "OLAK5uy_album001",
		"title": "Album - After Hours",
		"channel": "Low Window",
		"thumbnails": [
			{"url": "https://i.ytimg.com/vi/alb_sm.jpg", "height": 180},
			{"url": "https://i.ytimg.com/vi/alb_lg.jpg", "height": 640}
		],
		"entries": [
			{
				"id": "trk001_after",
				"title": "Blue Hour",
				"duration": 252,
				"url": "https://music.youtube.com/watch?v=trk001_after"
			},
			{
				"id": "trk002_space",
				"title": "The Space Between",
				"channel": "North Arcade",
				"duration": 306,
				"url": "https://music.youtube.com/watch?v=trk002_space"
			}
		]
	}`)

	album, tracks, err := ParseMusicAlbumJSON(albumJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing album JSON: %v", err)
	}
	if album == nil || album.Title != "After Hours" {
		t.Errorf("expected album title 'After Hours', got %q", album.Title)
	}
	if album.ThumbnailURL != "https://i.ytimg.com/vi/alb_lg.jpg" {
		t.Errorf("expected highest resolution cover art, got %q", album.ThumbnailURL)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if tracks[0].Channel != "Low Window" {
		t.Errorf("expected default album artist on track 0, got %q", tracks[0].Channel)
	}
	if tracks[1].Channel != "North Arcade" {
		t.Errorf("expected track-specific artist on track 1, got %q", tracks[1].Channel)
	}
	if tracks[0].Description != "After Hours" {
		t.Errorf("expected collection name 'After Hours' in description, got %q", tracks[0].Description)
	}

	// 2. Empty / null album
	albEmpty, trkEmpty, err := ParseMusicAlbumJSON([]byte("   null   "))
	if err != nil {
		t.Fatalf("unexpected error on null album: %v", err)
	}
	if albEmpty == nil || len(trkEmpty) != 0 {
		t.Errorf("expected empty album and 0 tracks on null, got %v / %d", albEmpty, len(trkEmpty))
	}
}
