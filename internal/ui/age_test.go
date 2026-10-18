package ui

import (
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		uploadDate  string
		publishedAt time.Time
		expected    string
	}{
		{
			name:     "empty values",
			expected: "",
		},
		{
			name:        "just now (30 seconds ago)",
			publishedAt: now.Add(-30 * time.Second),
			expected:    "just now",
		},
		{
			name:        "1 minute ago",
			publishedAt: now.Add(-65 * time.Second),
			expected:    "1 minute ago",
		},
		{
			name:        "20 minutes ago",
			publishedAt: now.Add(-20 * time.Minute),
			expected:    "20 minutes ago",
		},
		{
			name:        "1 hour ago",
			publishedAt: now.Add(-70 * time.Minute),
			expected:    "1 hour ago",
		},
		{
			name:        "5 hours ago",
			publishedAt: now.Add(-5 * time.Hour),
			expected:    "5 hours ago",
		},
		{
			name:        "1 day ago",
			publishedAt: now.Add(-26 * time.Hour),
			expected:    "1 day ago",
		},
		{
			name:        "3 days ago",
			publishedAt: now.Add(-3 * 24 * time.Hour),
			expected:    "3 days ago",
		},
		{
			name:        "1 week ago",
			publishedAt: now.Add(-8 * 24 * time.Hour),
			expected:    "1 week ago",
		},
		{
			name:        "2 weeks ago",
			publishedAt: now.Add(-15 * 24 * time.Hour),
			expected:    "2 weeks ago",
		},
		{
			name:        "1 month ago",
			publishedAt: now.Add(-35 * 24 * time.Hour),
			expected:    "1 month ago",
		},
		{
			name:        "4 months ago",
			publishedAt: now.Add(-125 * 24 * time.Hour),
			expected:    "4 months ago",
		},
		{
			name:        "1 year ago",
			publishedAt: now.Add(-400 * 24 * time.Hour),
			expected:    "1 year ago",
		},
		{
			name:        "3 years ago",
			publishedAt: now.Add(-1100 * 24 * time.Hour),
			expected:    "3 years ago",
		},
		{
			name:       "from upload date string",
			uploadDate: "20261015", // 3 days ago relative to 2026-10-18
			expected:   "3 days ago",
		},
		{
			name:       "invalid upload date string",
			uploadDate: "invalid",
			expected:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := FormatAge(tc.uploadDate, tc.publishedAt, now)
			if actual != tc.expected {
				t.Errorf("FormatAge() = %q, want %q", actual, tc.expected)
			}
		})
	}
}
