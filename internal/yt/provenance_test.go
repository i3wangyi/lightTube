package yt

import (
	"testing"
)

func TestProvenanceClassification(t *testing.T) {
	// 1. A canonical watch URL from a Shorts tab is a Short
	shortFromTab := Video{
		ID:    "shortFromTab",
		Title: "Quick Tip",
		URL:   "https://www.youtube.com/watch?v=shortFromTab",
	}
	if !IsShort(shortFromTab, ProvenanceShortsTab) {
		t.Errorf("expected canonical watch URL from Shorts tab to be classified as Short")
	}

	// 2. A canonical watch URL from Shorts search filter is a Short
	shortFromSearch := Video{
		ID:    "shortSearch1",
		Title: "Craft in 60s",
		URL:   "https://www.youtube.com/watch?v=shortSearch1",
	}
	if !IsShort(shortFromSearch, ProvenanceShortsSearch) {
		t.Errorf("expected canonical watch URL from Shorts search filter to be classified as Short")
	}

	// 3. A vertical video from a normal feed is NOT a Short
	verticalFeedVideo := Video{
		ID:          "vertFeedVid1",
		Title:       "Vertical Video In Feed (1080x1920)",
		URL:         "https://www.youtube.com/watch?v=vertFeedVid1",
		Duration:    45,
		Description: "A vertical orientation video uploaded to standard channel feed",
	}
	if IsShort(verticalFeedVideo, ProvenanceNormalFeed) {
		t.Errorf("expected vertical video from normal feed NOT to be classified as Short")
	}

	// 4. A short duration video from normal search is NOT a Short
	shortNormalSearch := Video{
		ID:       "normSearch1",
		Title:    "30 Second Clip",
		URL:      "https://www.youtube.com/watch?v=normSearch1",
		Duration: 30,
	}
	if IsShort(shortNormalSearch, ProvenanceSearch) {
		t.Errorf("expected short duration video from normal search NOT to be classified as Short")
	}

	// 5. Explicit /shorts/ URL without provenance specified is a Short
	explicitURL := Video{
		ID:    "expShort123",
		Title: "Explicit Short URL",
		URL:   "https://www.youtube.com/shorts/expShort123",
	}
	if !IsShort(explicitURL, "") {
		t.Errorf("expected explicit /shorts/ URL to be classified as Short")
	}
}

func TestExtractFullVideoSourceLink(t *testing.T) {
	// Case 1: Real source link in description
	descWithLink := "Thanks for watching! Watch the full 30 minute documentary here: https://www.youtube.com/watch?v=fullDocu001 or youtu.be"
	link := ExtractFullVideoSourceLink(descWithLink, "")
	if link != "https://www.youtube.com/watch?v=fullDocu001" {
		t.Errorf("expected extracted full video link, got %q", link)
	}

	// Case 2: Source link provided via relatedVideoID parameter
	linkRelated := ExtractFullVideoSourceLink("No link in description", "relDoc00001")
	if linkRelated != "https://www.youtube.com/watch?v=relDoc00001" {
		t.Errorf("expected related video link, got %q", linkRelated)
	}

	// Case 3: No real source link (just matching title or topic text)
	descNoLink := "Check out our full documentary titled 'The Quiet Craft of Making Things' on our channel!"
	linkNone := ExtractFullVideoSourceLink(descNoLink, "")
	if linkNone != "" {
		t.Errorf("expected empty link when no URL or ID is present, got %q", linkNone)
	}
}
