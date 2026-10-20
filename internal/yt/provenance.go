package yt

import (
	"net/url"
	"regexp"
	"strings"
)

// ProvenanceKind denotes where a video record was discovered or fetched from.
type ProvenanceKind string

const (
	ProvenanceNormalFeed   ProvenanceKind = "normal_feed"
	ProvenanceSearch       ProvenanceKind = "search"
	ProvenanceShortsTab    ProvenanceKind = "shorts_tab"
	ProvenanceShortsSearch ProvenanceKind = "shorts_search"
	ProvenanceShortsURL    ProvenanceKind = "shorts_url"
)

// IsShort reports whether a video qualifies as a Short strictly by provenance.
// A canonical watch URL returned by a Shorts tab or a Shorts search filter is a Short.
// A vertical video from a normal feed or normal video search is NOT a Short.
func IsShort(v Video, kind ProvenanceKind) bool {
	switch kind {
	case ProvenanceShortsTab, ProvenanceShortsSearch, ProvenanceShortsURL:
		return true
	default:
		// Also classify by explicit /shorts/ in URL if the origin is not normal feed/search
		if kind == "" && strings.Contains(v.URL, "/shorts/") {
			return true
		}
		return false
	}
}

var watchURLRegex = regexp.MustCompile(`https?://(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})`)

// ExtractFullVideoSourceLink looks for a real YouTube video link in the Short's description
// or related video identifier. It never matches by title or topic.
func ExtractFullVideoSourceLink(description string, relatedVideoID string) string {
	if relatedVideoID != "" {
		trimmed := strings.TrimSpace(relatedVideoID)
		if idPattern.MatchString(trimmed) {
			return "https://www.youtube.com/watch?v=" + trimmed
		}
	}

	matches := watchURLRegex.FindStringSubmatch(description)
	if len(matches) >= 2 {
		return "https://www.youtube.com/watch?v=" + matches[1]
	}

	// Also check raw URLs that might have query parameters or shorts paths
	for _, word := range strings.Fields(description) {
		if strings.Contains(word, "youtube.com/watch") || strings.Contains(word, "youtu.be/") {
			if u, err := url.Parse(word); err == nil {
				if id, err := ExtractVideoID(u.String()); err == nil {
					return "https://www.youtube.com/watch?v=" + id
				}
			}
		}
	}

	return ""
}
