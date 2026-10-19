// Package platform provides OS-specific integrations for power detection,
// font locations, cookie extraction browsers, and data storage directories.
package platform

import "strings"

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
