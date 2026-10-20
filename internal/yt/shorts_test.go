package yt

import (
	"errors"
	"strings"
	"testing"
)

func TestParseShortsTabJSON(t *testing.T) {
	// 1. Valid response with entries
	validJSON := []byte(`{
		"id": "UCtestChannel000000000001",
		"title": "Public Science - Shorts",
		"entries": [
			{
				"id": "shortAlpha01",
				"title": "Quantum Levitation in 30 Seconds",
				"channel": "Public Science",
				"channel_id": "UCtestChannel000000000001",
				"duration": 32,
				"view_count": 142000,
				"url": "https://www.youtube.com/shorts/shortAlpha01",
				"description": "Full lecture: https://www.youtube.com/watch?v=fullLec0001"
			},
			{
				"id": "shortBeta002",
				"title": "Superconductors at Work",
				"uploader": "Public Science",
				"uploader_id": "UCtestChannel000000000001",
				"duration": 45,
				"view_count": 89000,
				"url": "https://www.youtube.com/watch?v=shortBeta002"
			}
		]
	}`)

	videos, err := ParseShortsTabJSON(validJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing valid shorts tab: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}
	if videos[0].ID != "shortAlpha01" || videos[0].Duration != 32 {
		t.Errorf("entry 0 mismatch: %+v", videos[0])
	}
	// Verify source link extraction on entry 0
	srcLink := ExtractFullVideoSourceLink(videos[0].Description, "")
	if srcLink != "https://www.youtube.com/watch?v=fullLec0001" {
		t.Errorf("expected source link from description, got %q", srcLink)
	}

	// 2. Empty response
	emptyJSON := []byte(`{"id": "UCempty", "entries": []}`)
	vEmpty, err := ParseShortsTabJSON(emptyJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing empty entries: %v", err)
	}
	if len(vEmpty) != 0 {
		t.Errorf("expected 0 entries for empty list, got %d", len(vEmpty))
	}

	// 3. Null / whitespace response
	vNull, err := ParseShortsTabJSON([]byte("   null   "))
	if err != nil {
		t.Fatalf("unexpected error on null: %v", err)
	}
	if len(vNull) != 0 {
		t.Errorf("expected 0 entries for null, got %d", len(vNull))
	}

	// 4. Invalid / corrupt JSON
	_, err = ParseShortsTabJSON([]byte(`{"id": "UCbad", "entries": [ { invalid json `))
	if err == nil {
		t.Errorf("expected error parsing corrupt JSON, got nil")
	}

	// 5. Changed upstream schema (e.g. unknown keys, nulls, missing expected fields)
	changedSchemaJSON := []byte(`{
		"new_upstream_field": "v2",
		"playlist_count": null,
		"entries": [
			{
				"id": "shortGamma03",
				"title": null,
				"extra_meta": {"nested": true},
				"view_count": null
			},
			{
				"unexpected_field": "no ID"
			}
		]
	}`)
	vChanged, err := ParseShortsTabJSON(changedSchemaJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing changed schema JSON: %v", err)
	}
	if len(vChanged) != 1 {
		t.Fatalf("expected 1 valid entry with ID, got %d", len(vChanged))
	}
	if vChanged[0].ID != "shortGamma03" {
		t.Errorf("expected ID 'shortGamma03', got %q", vChanged[0].ID)
	}
}

func TestParseShortsSearchJSON(t *testing.T) {
	searchJSON := []byte(`{
		"entries": [
			{
				"id": "shortSrch001",
				"title": "Origami in 45 Seconds",
				"channel": "Paper Craft",
				"channel_id": "UCpaperCraft0000000001",
				"duration": 45,
				"view_count": 55000,
				"url": "https://www.youtube.com/watch?v=shortSrch001"
			}
		]
	}`)

	results, err := ParseShortsSearchJSON(searchJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing shorts search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "shortSrch001" {
		t.Errorf("result ID mismatch: got %q", results[0].ID)
	}
	if !IsShort(results[0], ProvenanceShortsSearch) {
		t.Errorf("expected search result to have Short classification with ProvenanceShortsSearch")
	}
}

func TestShortsSearchURL(t *testing.T) {
	urlStr := ShortsSearchURL("origami art")
	if !strings.Contains(urlStr, "search_query=origami+art") && !strings.Contains(urlStr, "search_query=origami%20art") {
		t.Errorf("expected escaped query in URL, got %q", urlStr)
	}
	if !strings.Contains(urlStr, "sp=EgIQCQ%3D%3D") {
		t.Errorf("expected sp=EgIQCQ%%3D%%3D parameter in URL, got %q", urlStr)
	}
	if strings.Contains(urlStr, "sp=EgIYAQ%3D%3D") {
		t.Errorf("URL should not contain under-4-min filter sp=EgIYAQ%%3D%%3D, got %q", urlStr)
	}
}

func TestMergedShortsSortsNewestAndKeepsChannelFailures(t *testing.T) {
	videos, err := mergeShortsResults([]shortsChannelResult{
		{channelID: "UCfixtureA", videos: []Video{{ID: "older_short", UploadDate: "20250101"}, {ID: "newer_short", UploadDate: "20260101"}}},
		{channelID: "UCfixtureB", videos: []Video{{ID: "newest_short", UploadDate: "20261001"}}, err: errors.New("fixture channel failure")},
	})
	if err == nil || !strings.Contains(err.Error(), "UCfixtureB") {
		t.Fatalf("channel failure was lost: %v", err)
	}
	if len(videos) != 3 || videos[0].ID != "newest_short" || videos[1].ID != "newer_short" || videos[2].ID != "older_short" {
		t.Fatalf("shorts are not newest first: %+v", videos)
	}
}
