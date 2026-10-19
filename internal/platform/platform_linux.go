//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultSysfsPowerSupply = "/sys/class/power_supply"

// OnBattery determines whether the device is running on battery power.
// It reports known=true when power supplies were found and read successfully.
func OnBattery() (onBattery bool, known bool) {
	return OnBatteryFrom(defaultSysfsPowerSupply)
}

// OnBatteryFrom checks sysfs power supplies in the specified directory.
func OnBatteryFrom(dir string) (bool, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, false
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
			return false, true
		case "0":
			hasOfflineMains = true
		}
	}

	if foundMains && hasOfflineMains {
		return true, true
	}

	return false, false
}

// FontFiles returns standard Noto font paths on Linux.
func FontFiles() []string {
	return []string{
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",
		"/usr/share/fonts/noto/NotoSans-Medium.ttf",
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	}
}

// DefaultBrowserSpec returns browser and keyring specification for Linux desktop environments.
func DefaultBrowserSpec() string {
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if strings.Contains(desktop, "KDE") {
		return "chrome+kwallet"
	}
	return "chrome+gnomekeyring"
}

// DataDir returns the standard application data directory on Linux ($XDG_DATA_HOME/lighttube).
func DataDir() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "lighttube")
}

// CacheDir returns the standard application cache directory on Linux ($XDG_CACHE_HOME/lighttube).
func CacheDir() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "lighttube")
}
