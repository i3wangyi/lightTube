package player

import "lighttube/internal/platform"

const defaultSysfsPowerSupply = "/sys/class/power_supply"

// DetectPower determines the system power source using platform detection.
// Per specification, it treats missing or unreadable mains supplies as AC.
func DetectPower() Power {
	onBat, known := platform.OnBattery()
	if known && onBat {
		return PowerBattery
	}
	return PowerAC
}

func detectPowerFrom(dir string) Power {
	onBat, known := platform.OnBatteryFrom(dir)
	if known && onBat {
		return PowerBattery
	}
	return PowerAC
}
