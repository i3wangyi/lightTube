package player

import (
	"lighttube/internal/embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayerBuildArgs(t *testing.T) {
	sockPath := "/tmp/test-lighttube.sock"
	appID := "test-app-id"

	formatAC := Format(PowerAC, ModeVideo)
	// The hwdec value depends on the OS; TestHWDecPerOS covers each OS.
	wantOldGPU := "--hwdec=" + HWDec(CodecPolicyOldGPU, embed.Caps{})
	formatBattery := Format(PowerBattery, ModeVideo)

	tests := []struct {
		name       string
		setupPower func(dir string)
		battery    bool
		opts       Options
		check      func(t *testing.T, args []string)
	}{
		{
			name: "video AC arguments",
			opts: Options{
				URL:   "https://youtube.com/watch?v=123",
				Title: "Test Video",
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, wantOldGPU)
				assertContains(t, args, "--vo=gpu")
				assertContains(t, args, "--force-window=immediate")
				assertContains(t, args, "--title=Test Video")
				assertContains(t, args, "--wayland-app-id=test-app-id")
				assertContains(t, args, "--input-ipc-server=/tmp/test-lighttube.sock")
				assertContains(t, args, "--ytdl-format="+formatAC)
				if args[len(args)-1] != "https://youtube.com/watch?v=123" {
					t.Errorf("last argument should be URL, got %q", args[len(args)-1])
				}
			},
		},
		{
			name:    "video battery arguments",
			battery: true,
			setupPower: func(dir string) {
				adp := filepath.Join(dir, "ADP1")
				_ = os.MkdirAll(adp, 0o755)
				_ = os.WriteFile(filepath.Join(adp, "type"), []byte("Mains\n"), 0o644)
				_ = os.WriteFile(filepath.Join(adp, "online"), []byte("0\n"), 0o644)
			},
			opts: Options{
				URL: "https://youtube.com/watch?v=123",
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--ytdl-format="+formatBattery)
				assertContains(t, args, "--force-window=immediate")
				// Title omitted when empty.
				for _, arg := range args {
					if strings.HasPrefix(arg, "--title=") {
						t.Errorf("did not expect title argument, got %q", arg)
					}
				}
			},
		},
		{
			name: "audio mode arguments",
			opts: Options{
				URL:   "https://youtube.com/watch?v=123",
				Audio: true,
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--ytdl-format=ba")
				assertContains(t, args, "--no-video")
				assertContains(t, args, "--force-window=no")
				for _, arg := range args {
					if arg == "--force-window=immediate" {
						t.Errorf("audio mode must not include --force-window=immediate")
					}
				}
			},
		},
		{
			name: "extra arguments included",
			opts: Options{
				URL:       "https://youtube.com/watch?v=123",
				ExtraArgs: []string{"--ytdl-raw-options=cookies=/data/cookies.txt"},
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--ytdl-raw-options=cookies=/data/cookies.txt")
				if args[len(args)-1] != "https://youtube.com/watch?v=123" {
					t.Errorf("last argument should be URL, got %q", args[len(args)-1])
				}
			},
		},
		{
			name: "policy auto with VP9 decode uses auto-safe hwdec",
			opts: Options{
				URL:    "https://youtube.com/watch?v=123",
				Policy: CodecPolicyAuto,
				Caps:   embed.Caps{H264: true, VP9: true},
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--hwdec=auto-safe")
			},
		},
		{
			name: "policy any uses auto-safe hwdec",
			opts: Options{
				URL:    "https://youtube.com/watch?v=123",
				Policy: CodecPolicyAny,
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--hwdec=auto-safe")
			},
		},
		{
			name: "policy old-gpu uses the OS hwdec",
			opts: Options{
				URL:    "https://youtube.com/watch?v=123",
				Policy: CodecPolicyOldGPU,
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, wantOldGPU)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysfsDir := t.TempDir()
			if tt.setupPower != nil {
				tt.setupPower(sysfsDir)
			}
			player := NewWithOptions(sockPath, sysfsDir, appID)
			// Set the power state for every case, so a Mac on battery
			// cannot change the AC expectations.
			power := PowerAC
			if tt.battery {
				power = PowerBattery
			}
			player.powerSource = func() Power { return power }
			args := player.BuildArgs(tt.opts)
			tt.check(t, args)
		})
	}
}

func assertContains(t *testing.T, slice []string, item string) {
	t.Helper()
	for _, s := range slice {
		if s == item {
			return
		}
	}
	t.Errorf("expected slice to contain %q, but got %v", item, slice)
}

func TestHWDecPerOS(t *testing.T) {
	cases := []struct {
		goos   string
		policy CodecPolicy
		want   string
	}{
		{"linux", CodecPolicyOldGPU, "vaapi"},
		{"darwin", CodecPolicyOldGPU, "auto-safe"},
		{"windows", CodecPolicyOldGPU, "auto-safe"},
		{"linux", CodecPolicyAny, "auto-safe"},
	}
	for _, c := range cases {
		if got := hwdecFor(c.goos, c.policy, embed.Caps{}); got != c.want {
			t.Errorf("hwdecFor(%q, %q) = %q, want %q", c.goos, c.policy, got, c.want)
		}
	}
}
