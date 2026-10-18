package player

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultSysfsPowerSupply = "/sys/class/power_supply"

// DetectPower determines the system power source by reading sysfs.
// Per specification, it treats missing or unreadable mains supplies as AC.
func DetectPower() Power {
	return detectPowerFrom(defaultSysfsPowerSupply)
}

func detectPowerFrom(dir string) Power {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Default to AC if sysfs directory cannot be accessed.
		return PowerAC
	}

	foundMains := false
	hasOfflineMains := false

	for _, entry := range entries {
		typeBytes, err := os.ReadFile(filepath.Join(dir, entry.Name(), "type"))
		if err != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(string(typeBytes)), "Mains") {
			continue
		}

		foundMains = true
		onlineBytes, err := os.ReadFile(filepath.Join(dir, entry.Name(), "online"))
		if err != nil {
			continue
		}

		switch strings.TrimSpace(string(onlineBytes)) {
		case "1":
			return PowerAC
		case "0":
			hasOfflineMains = true
		}
	}

	// Treat as battery only if mains supply was found and explicitly reported offline.
	if foundMains && hasOfflineMains {
		return PowerBattery
	}

	return PowerAC
}
