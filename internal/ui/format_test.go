package ui

import (
	"testing"
	"time"
)

func TestFormatViewCount(t *testing.T) {
	tests := []struct {
		views int64
		want  string
	}{
		{0, ""},
		{-5, ""},
		{1, "1"},
		{999, "999"},
		{1000, "1K"},
		{1200, "1.2K"},
		{9900, "9.9K"},
		{10000, "10K"},
		{34000, "34K"},
		{100000, "100K"},
		{999000, "999K"},
		{1000000, "1M"},
		{1200000, "1.2M"},
		{10000000, "10M"},
		{34000000, "34M"},
		{1000000000, "1B"},
		{3400000000, "3.4B"},
	}

	for _, tt := range tests {
		got := FormatViewCount(tt.views)
		if got != tt.want {
			t.Errorf("FormatViewCount(%d) = %q, want %q", tt.views, got, tt.want)
		}
	}
}

func TestFormatMetaLine(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	threeDaysAgo := now.Add(-3 * 24 * time.Hour)

	// Both views and age
	got := FormatMetaLine(1200000, "", threeDaysAgo, now)
	if got != "1.2M views · 3 days ago" {
		t.Errorf("expected '1.2M views · 3 days ago', got %q", got)
	}

	// Single view
	got = FormatMetaLine(1, "", threeDaysAgo, now)
	if got != "1 view · 3 days ago" {
		t.Errorf("expected '1 view · 3 days ago', got %q", got)
	}

	// Views only (zero time)
	got = FormatMetaLine(34000, "", time.Time{}, now)
	if got != "34K views" {
		t.Errorf("expected '34K views', got %q", got)
	}

	// Age only (zero views)
	got = FormatMetaLine(0, "", threeDaysAgo, now)
	if got != "3 days ago" {
		t.Errorf("expected '3 days ago', got %q", got)
	}

	// Neither
	got = FormatMetaLine(0, "", time.Time{}, now)
	if got != "" {
		t.Errorf("expected '', got %q", got)
	}
}
