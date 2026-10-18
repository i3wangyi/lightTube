package player

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPowerFrom(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		want        Power
		overrideDir func(base string) string
	}{
		{
			name: "missing directory defaults to AC",
			overrideDir: func(base string) string {
				return filepath.Join(base, "non-existent-dir")
			},
			want: PowerAC,
		},
		{
			name:  "empty directory defaults to AC",
			setup: func(t *testing.T, dir string) {},
			want:  PowerAC,
		},
		{
			name: "only battery present defaults to AC",
			setup: func(t *testing.T, dir string) {
				batDir := filepath.Join(dir, "BAT0")
				if err := os.MkdirAll(batDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(batDir, "type"), []byte("Battery\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: PowerAC,
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
			want: PowerAC,
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
			want: PowerBattery,
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
			want: PowerAC,
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
			want: PowerBattery,
		},
		{
			name: "mains without online file defaults to AC",
			setup: func(t *testing.T, dir string) {
				adpDir := filepath.Join(dir, "ADP1")
				if err := os.MkdirAll(adpDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(adpDir, "type"), []byte("Mains\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: PowerAC,
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
			want: PowerAC,
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

			got := detectPowerFrom(targetDir)
			if got != tt.want {
				t.Errorf("detectPowerFrom(%q) = %v; want %v", targetDir, got, tt.want)
			}
		})
	}
}

func TestDetectPowerLive(t *testing.T) {
	// Smoke test against the host system's real sysfs.
	power := DetectPower()
	if power != PowerAC && power != PowerBattery {
		t.Fatalf("unexpected power value: %v", power)
	}
}
