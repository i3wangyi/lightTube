//go:build darwin

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
)

// OnBattery determines whether the device is running on battery power by parsing pmset.
func OnBattery() (bool, bool) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return false, false
	}
	return parsePmsetBatt(string(out))
}

// OnBatteryFrom checks battery status (delegates to OnBattery on macOS).
func OnBatteryFrom(dir string) (bool, bool) {
	return OnBattery()
}

// FontFiles returns standard macOS PingFang and San Francisco font paths.
func FontFiles() []string {
	return []string{
		"/System/Library/Fonts/SFNS.ttf",
		"/System/Library/Fonts/SFNSText.ttf",
		"/System/Library/Fonts/SFPro.ttf",
		"/System/Library/Fonts/PingFang.ttc",
	}
}

// DefaultBrowserSpec returns the default cookie browser for macOS.
func DefaultBrowserSpec() string {
	return "chrome"
}

// DataDir returns the application data directory on macOS (os.UserConfigDir()/lighttube).
func DataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = os.TempDir()
	}
	return filepath.Join(configDir, "lighttube")
}

// CacheDir returns the application cache directory on macOS (os.UserCacheDir()/lighttube).
func CacheDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "lighttube")
}
