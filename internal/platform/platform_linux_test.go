//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnBatteryFrom(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		overrideDir func(base string) string
		wantBat     bool
		wantKnown   bool
	}{
		{
			name: "missing directory defaults to unknown",
			overrideDir: func(base string) string {
				return filepath.Join(base, "non-existent-dir")
			},
			wantBat:   false,
			wantKnown: false,
		},
		{
			name:      "empty directory defaults to unknown",
			setup:     func(t *testing.T, dir string) {},
			wantBat:   false,
			wantKnown: false,
		},
		{
			name: "only battery present defaults to unknown",
			setup: func(t *testing.T, dir string) {
				batDir := filepath.Join(dir, "BAT0")
				if err := os.MkdirAll(batDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(batDir, "type"), []byte("Battery\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   false,
			wantKnown: false,
		},
		{
			name: "mains online reports AC",
			setup: func(t *testing.T, dir string) {
				adpDir := filepath.Join(dir, "ADP1")
				if err := os.MkdirAll(adpDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "online"), []byte("1\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   false,
			wantKnown: true,
		},
		{
			name: "mains offline reports Battery",
			setup: func(t *testing.T, dir string) {
				adpDir := filepath.Join(dir, "ADP1")
				batDir := filepath.Join(dir, "BAT0")
				if err := os.MkdirAll(adpDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(batDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "online"), []byte("0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(batDir, "type"), []byte("Battery\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   true,
			wantKnown: true,
		},
		{
			name: "multiple mains with one online reports AC",
			setup: func(t *testing.T, dir string) {
				m1 := filepath.Join(dir, "ADP1")
				m2 := filepath.Join(dir, "USBPD")
				if err := os.MkdirAll(m1, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(m2, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m1, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m1, "online"), []byte("0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m2, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m2, "online"), []byte("1\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   false,
			wantKnown: true,
		},
		{
			name: "multiple mains with all offline reports Battery",
			setup: func(t *testing.T, dir string) {
				m1 := filepath.Join(dir, "ADP1")
				m2 := filepath.Join(dir, "USBPD")
				if err := os.MkdirAll(m1, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(m2, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m1, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m1, "online"), []byte("0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m2, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m2, "online"), []byte("0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   true,
			wantKnown: true,
		},
		{
			name: "mains without online file defaults to unknown",
			setup: func(t *testing.T, dir string) {
				adpDir := filepath.Join(dir, "ADP1")
				if err := os.MkdirAll(adpDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   false,
			wantKnown: false,
		},
		{
			name: "whitespace and case insensitive matching",
			setup: func(t *testing.T, dir string) {
				adpDir := filepath.Join(dir, "ADP1")
				if err := os.MkdirAll(adpDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "type"), []byte("  mAiNs \n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "online"), []byte(" 1 \n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantBat:   false,
			wantKnown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			targetDir := dir
			if tt.overrideDir != nil {
				targetDir = tt.overrideDir(dir)
			}

			gotBat, gotKnown := OnBatteryFrom(targetDir)
			if gotBat != tt.wantBat || gotKnown != tt.wantKnown {
				t.Errorf("onBatteryFrom(%q) = (%v, %v); want (%v, %v)", targetDir, gotBat, gotKnown, tt.wantBat, tt.wantKnown)
			}
		})
	}
}

func TestOnBatteryLive(t *testing.T) {
	// Smoke test against the host system's real sysfs.
	_, _ = OnBattery()
}

func TestDefaultBrowserSpec(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	if got := DefaultBrowserSpec(); got != "chrome+kwallet" {
		t.Errorf("DefaultBrowserSpec() with KDE = %q, want chrome+kwallet", got)
	}

	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	if got := DefaultBrowserSpec(); got != "chrome+gnomekeyring" {
		t.Errorf("DefaultBrowserSpec() with GNOME = %q, want chrome+gnomekeyring", got)
	}
}

func TestDataAndCacheDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")

	if got := DataDir(); got != "/custom/data/lighttube" {
		t.Errorf("DataDir() = %q, want /custom/data/lighttube", got)
	}
	if got := CacheDir(); got != "/custom/cache/lighttube" {
		t.Errorf("CacheDir() = %q, want /custom/cache/lighttube", got)
	}
}
