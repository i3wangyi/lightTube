package main

import (
	"errors"
	"fmt"
	"os"

	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"lighttube/internal/embed"
	"lighttube/internal/platform"
	"lighttube/internal/player"
	"lighttube/internal/store"
	"lighttube/internal/yt"
)

func runDoctor(env *appEnv) error {
	hasFail := false
	report := func(status, name, details string) {
		fmt.Fprintf(env.stdout, "[%s] %s: %s\n", status, name, details)
		if status == "FAIL" {
			hasFail = true
		}
	}

	// 1. mpv on PATH and version
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		report("FAIL", "mpv", "not found on PATH (please install mpv)")
	} else {
		out, err := exec.Command(mpvPath, "--version").Output()
		ver := "mpv"
		if err == nil {
			lines := strings.Split(string(out), "\n")
			if len(lines) > 0 {
				ver = strings.TrimSpace(lines[0])
			}
		}
		report("OK", "mpv", fmt.Sprintf("%s (%s)", ver, mpvPath))
	}

	// 2. libmpv.so.2 loadable
	if err := embed.CheckLibmpv(); err != nil {
		report("FAIL", "libmpv", fmt.Sprintf("cannot load libmpv.so.2: %v", err))
	} else {
		report("OK", "libmpv", "libmpv.so.2 loadable")
	}

	// 3. yt-dlp on PATH, version and age
	ytdlpPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		report("FAIL", "yt-dlp", "not found on PATH (please install yt-dlp)")
	} else {
		out, err := exec.Command(ytdlpPath, "--version").Output()
		if err != nil {
			report("FAIL", "yt-dlp", fmt.Sprintf("failed running --version: %v", err))
		} else {
			verStr := strings.TrimSpace(string(out))
			tVer, pErr := time.Parse("2006.01.02", verStr)
			ageDays := -1
			if pErr == nil {
				ageDays = int(time.Since(tVer).Hours() / 24)
			}

			if ageDays > 60 {
				hint := "yt-dlp -U or package manager"
				switch runtime.GOOS {
				case "linux":
					hint = "yt-dlp -U or package manager (e.g. pacman, apt)"
				case "darwin":
					hint = "yt-dlp -U or brew upgrade yt-dlp"
				case "windows":
					hint = "yt-dlp -U or winget upgrade yt-dlp"
				}
				report("WARN", "yt-dlp", fmt.Sprintf("%s (%d days old, update hint: %s)", verStr, ageDays, hint))
			} else if ageDays >= 0 {
				report("OK", "yt-dlp", fmt.Sprintf("%s (%d days old)", verStr, ageDays))
			} else {
				report("OK", "yt-dlp", verStr)
			}
		}
	}

	// 4. JavaScript runtime for yt-dlp
	var jsFound []string
	for _, name := range []string{"deno", "node", "bun", "quickjs", "qjs"} {
		if p, err := exec.LookPath(name); err == nil {
			jsFound = append(jsFound, fmt.Sprintf("%s (%s)", name, p))
		}
	}
	if len(jsFound) == 0 {
		report("WARN", "JavaScript runtime", "none found on PATH; yt-dlp may fail on YouTube signature extraction (install deno or node)")
	} else {
		report("OK", "JavaScript runtime", strings.Join(jsFound, ", "))
	}

	// 5. GPU decode caps and auto codec policy
	caps, err := embed.DecodeCaps()
	if err != nil {
		report("WARN", "GPU decode", fmt.Sprintf("probe failed: %v, auto codec policy: %s", err, player.CodecPolicyOldGPU))
	} else {
		autoPol := player.PolicyAutoPicks(caps)
		var codecs []string
		if caps.H264 {
			codecs = append(codecs, "H.264")
		}
		if caps.HEVC {
			codecs = append(codecs, "HEVC")
		}
		if caps.VP9 {
			codecs = append(codecs, "VP9")
		}
		if caps.AV1 {
			codecs = append(codecs, "AV1")
		}
		cStr := strings.Join(codecs, ", ")
		if cStr == "" {
			cStr = "none"
		}
		driverStr := ""
		if caps.Driver != "" {
			driverStr = fmt.Sprintf(" (driver: %s)", caps.Driver)
		}
		report("OK", "GPU decode", fmt.Sprintf("%s%s, auto codec policy: %s", cStr, driverStr, autoPol))
	}

	// 6. Settings file values
	dataDir := platform.DataDir()
	if env.store != nil {
		dataDir = env.store.DataDir()
	}
	settings, _ := store.LoadSettings(dataDir)
	settingsPath := filepath.Join(dataDir, "settings.json")
	if _, statErr := os.Stat(settingsPath); statErr != nil {
		report("OK", "Settings", fmt.Sprintf("codec_policy=%s, player=%s, browser_spec=%s (using defaults)",
			settings.CodecPolicy, settings.Player, settings.BrowserSpec))
	} else {
		report("OK", "Settings", fmt.Sprintf("codec_policy=%s, player=%s, browser_spec=%s",
			settings.CodecPolicy, settings.Player, settings.BrowserSpec))
	}

	// 7. Login state (never print cookie values)
	acc := env.account
	if acc == nil {
		acc = yt.NewAccount()
	}
	if acc.SignedIn() {
		report("OK", "Login state", "signed in")
	} else {
		report("OK", "Login state", "signed out")
	}

	// 8. Data and cache dirs
	dDir := platform.DataDir()
	if env.store != nil {
		dDir = env.store.DataDir()
	}
	cDir := platform.CacheDir()
	report("OK", "Directories", fmt.Sprintf("data=%s, cache=%s", dDir, cDir))

	if hasFail {
		return errors.New("doctor found one or more failures")
	}
	return nil
}
