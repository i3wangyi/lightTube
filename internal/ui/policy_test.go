package ui

import (
	"testing"

	"lighttube/internal/player"
)

func TestFormatPolicyLabel(t *testing.T) {
	tests := []struct {
		name     string
		power    player.Power
		mode     player.Mode
		expected string
	}{
		{
			name:     "video on AC",
			power:    player.PowerAC,
			mode:     player.ModeVideo,
			expected: "H.264 ≤1080p · AC",
		},
		{
			name:     "video on battery",
			power:    player.PowerBattery,
			mode:     player.ModeVideo,
			expected: "H.264 ≤720p · Battery",
		},
		{
			name:     "audio on AC",
			power:    player.PowerAC,
			mode:     player.ModeAudio,
			expected: "Audio · AC",
		},
		{
			name:     "audio on battery",
			power:    player.PowerBattery,
			mode:     player.ModeAudio,
			expected: "Audio · Battery",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := FormatPolicyLabel(tc.power, tc.mode)
			if actual != tc.expected {
				t.Errorf("FormatPolicyLabel() = %q, want %q", actual, tc.expected)
			}
		})
	}
}
