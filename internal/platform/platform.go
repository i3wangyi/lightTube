// Package platform provides OS-specific integrations for power detection,
// font locations, cookie extraction browsers, and data storage directories.
package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// ExtendPath appends the OS's extra tool directories (extraPathDirs) to
// PATH when they are missing, so mpv and yt-dlp resolve when the app
// starts from a desktop launcher with a minimal PATH.
func ExtendPath() {
	os.Setenv("PATH", extendedPath(os.Getenv("PATH"), extraPathDirs()))
}

func extendedPath(cur string, extra []string) string {
	have := map[string]bool{}
	for _, d := range filepath.SplitList(cur) {
		have[d] = true
	}
	out := cur
	for _, d := range extra {
		if have[d] {
			continue
		}
		have[d] = true
		if out == "" {
			out = d
		} else {
			out += string(os.PathListSeparator) + d
		}
	}
	return out
}

// parsePmsetBatt parses the output of macOS 'pmset -g batt'.
// It returns (onBattery, known).
func parsePmsetBatt(out string) (bool, bool) {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "'battery power'") || strings.Contains(lower, "drawing from 'battery") {
		return true, true
	}
	if strings.Contains(lower, "'ac power'") || strings.Contains(lower, "drawing from 'ac") {
		return false, true
	}
	return false, false
}
