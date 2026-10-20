//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
)

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// OnBattery determines whether the device is running on battery power via GetSystemPowerStatus.
// extraPathDirs is empty: desktop launchers here inherit the session PATH.
func extraPathDirs() []string { return nil }

func OnBattery() (bool, bool) {
	var sps systemPowerStatus
	r1, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&sps)))
	if r1 == 0 {
		return false, false
	}
	switch sps.ACLineStatus {
	case 0:
		return true, true
	case 1:
		return false, true
	default:
		return false, false
	}
}

// OnBatteryFrom checks battery status (delegates to OnBattery on Windows).
func OnBatteryFrom(dir string) (bool, bool) {
	return OnBattery()
}

// FontFiles returns standard Windows font paths for Microsoft YaHei and Segoe UI.
func FontFiles() []string {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	fonts := filepath.Join(windir, "Fonts")
	return []string{
		filepath.Join(fonts, "segoeui.ttf"),
		filepath.Join(fonts, "msyh.ttc"),
	}
}

// DefaultBrowserSpec returns the default cookie browser for Windows (Firefox).
func DefaultBrowserSpec() string {
	return "firefox"
}

// DataDir returns the application data directory on Windows (os.UserConfigDir()/lighttube).
func DataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = os.TempDir()
	}
	return filepath.Join(configDir, "lighttube")
}

// CacheDir returns the application cache directory on Windows (os.UserCacheDir()/lighttube).
func CacheDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "lighttube")
}
