package ui

import (
	"fmt"

	"lighttube/internal/player"
)

// FormatPolicyLabel returns the human-readable policy string such as
// "H.264 ≤1080p · AC" or "H.264 ≤720p · Battery" for display in the interface header.
func FormatPolicyLabel(power player.Power, mode player.Mode) string {
	powerStr := power.String()
	if mode == player.ModeAudio {
		return fmt.Sprintf("Audio · %s", powerStr)
	}
	if power == player.PowerBattery {
		return fmt.Sprintf("H.264 ≤720p · %s", powerStr)
	}
	return fmt.Sprintf("H.264 ≤1080p · %s", powerStr)
}
