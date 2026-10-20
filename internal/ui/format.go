package ui

import (
	"fmt"
	"strings"
	"time"

	"lighttube/internal/yt"
)

func formatPlaybackTime(position, duration float64) string {
	positionText := yt.FormatDuration(int(position))
	if duration <= 0 {
		return positionText
	}
	return fmt.Sprintf("%s / %s", positionText, yt.FormatDuration(int(duration)))
}

// FormatViewCount formats view counts according to YouTube convention:
// 999, 1.2K, 34K, 1.2M, 3.4B.
func FormatViewCount(views int64) string {
	if views <= 0 {
		return ""
	}
	if views < 1000 {
		return fmt.Sprintf("%d", views)
	}
	if views < 10000 {
		// 1.0K to 9.9K
		v := float64(views) / 1000.0
		s := fmt.Sprintf("%.1fK", v)
		if strings.HasSuffix(s, ".0K") {
			return fmt.Sprintf("%dK", views/1000)
		}
		return s
	}
	if views < 1000000 {
		// 10K to 999K
		return fmt.Sprintf("%dK", views/1000)
	}
	if views < 10000000 {
		// 1.0M to 9.9M
		v := float64(views) / 1000000.0
		s := fmt.Sprintf("%.1fM", v)
		if strings.HasSuffix(s, ".0M") {
			return fmt.Sprintf("%dM", views/1000000)
		}
		return s
	}
	if views < 1000000000 {
		// 10M to 999M
		return fmt.Sprintf("%dM", views/1000000)
	}

	// 1.0B+
	v := float64(views) / 1000000000.0
	s := fmt.Sprintf("%.1fB", v)
	if strings.HasSuffix(s, ".0B") {
		return fmt.Sprintf("%dB", views/1000000000)
	}
	return s
}

// FormatMetaLine formats the "1.2M views · 3 days ago" meta string.
// It omits parts that are unknown.
func FormatMetaLine(views int64, uploadDate string, publishedAt time.Time, now time.Time) string {
	var viewsStr string
	if views > 0 {
		vCount := FormatViewCount(views)
		if views == 1 {
			viewsStr = "1 view"
		} else if vCount != "" {
			viewsStr = vCount + " views"
		}
	}

	ageStr := FormatAge(uploadDate, publishedAt, now)

	if viewsStr != "" && ageStr != "" {
		return viewsStr + " · " + ageStr
	}
	if viewsStr != "" {
		return viewsStr
	}
	return ageStr
}
