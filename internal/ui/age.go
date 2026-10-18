package ui

import (
	"fmt"
	"time"
)

// FormatAge converts an upload date string (YYYYMMDD) or a publishedAt time
// into a human-readable relative age string such as "3 days ago" or "1 year ago".
func FormatAge(uploadDate string, publishedAt time.Time, now time.Time) string {
	t := publishedAt
	if t.IsZero() && len(uploadDate) == 8 {
		parsed, err := time.Parse("20060102", uploadDate)
		if err == nil {
			t = parsed
		}
	}

	if t.IsZero() {
		return ""
	}

	diff := now.Sub(t)
	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins <= 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours <= 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		if days <= 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	if diff < 30*24*time.Hour {
		weeks := int(diff.Hours() / (24 * 7))
		if weeks <= 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}
	if diff < 365*24*time.Hour {
		months := int(diff.Hours() / (24 * 30.4375))
		if months <= 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}

	years := int(diff.Hours() / (24 * 365.25))
	if years <= 1 {
		return "1 year ago"
	}
	return fmt.Sprintf("%d years ago", years)
}
