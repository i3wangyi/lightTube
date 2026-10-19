package player

import (
	"testing"
)

func TestDetectPowerLive(t *testing.T) {
	// Smoke test against the host system's real power detection.
	power := DetectPower()
	if power != PowerAC && power != PowerBattery {
		t.Fatalf("unexpected power value: %v", power)
	}
}
