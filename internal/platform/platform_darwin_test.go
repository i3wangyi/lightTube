//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// macOS keeps caches under ~/Library/Caches and ignores XDG_CACHE_HOME.
func TestDarwinDirs(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got, want := CacheDir(), filepath.Join(home, "Library", "Caches", "lighttube"); got != want {
		t.Errorf("CacheDir() = %q, want %q", got, want)
	}
	if got, want := DataDir(), filepath.Join(home, "Library", "Application Support", "lighttube"); got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
	if got := extraPathDirs(); len(got) != 2 || got[0] != "/opt/homebrew/bin" || got[1] != "/usr/local/bin" {
		t.Errorf("extraPathDirs() = %v", got)
	}
}
