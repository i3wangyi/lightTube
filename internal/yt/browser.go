package yt

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"lighttube/internal/platform"
)

// BrowserProfile represents a discovered browser profile with signed-in Google account metadata.
type BrowserProfile struct {
	Browser     string `json:"browser"`      // "chrome", "chromium", "brave", "edge", "firefox"
	ProfileKey  string `json:"profile_key"`  // "Default", "Profile 1", or path
	Name        string `json:"name"`         // Display name
	UserName    string `json:"user_name"`    // Google account name or email
	BrowserSpec string `json:"browser_spec"` // yt-dlp format: browser+keyring:profile
}

// DiscoverBrowserProfiles searches standard browser config paths for profiles in the user's home dir.
func DiscoverBrowserProfiles() []BrowserProfile {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return DiscoverBrowserProfilesFrom(home)
}

// DiscoverBrowserProfilesFrom searches browser config paths relative to a given home directory.
func DiscoverBrowserProfilesFrom(homeDir string) []BrowserProfile {
	var profiles []BrowserProfile

	// Determine platform keyring suffix (e.g. "+gnomekeyring" on Linux)
	defSpec := platform.DefaultBrowserSpec()
	keyring := ""
	if idx := strings.IndexByte(defSpec, '+'); idx != -1 {
		keyring = defSpec[idx:]
	}

	// Chromium-based browsers: Chrome, Chromium, Brave, Edge
	chromiumConfigs := []struct {
		browser string
		relPath string
	}{
		{"chrome", filepath.Join(".config", "google-chrome", "Local State")},
		{"chromium", filepath.Join(".config", "chromium", "Local State")},
		{"brave", filepath.Join(".config", "BraveSoftware", "Brave-Browser", "Local State")},
		{"edge", filepath.Join(".config", "microsoft-edge", "Local State")},
	}

	for _, cfg := range chromiumConfigs {
		localStatePath := filepath.Join(homeDir, cfg.relPath)
		profs := parseLocalStateProfiles(localStatePath, cfg.browser, keyring)
		profiles = append(profiles, profs...)
	}

	// Firefox: profiles.ini
	firefoxIniPath := filepath.Join(homeDir, ".mozilla", "firefox", "profiles.ini")
	ffProfs := parseFirefoxProfiles(firefoxIniPath)
	profiles = append(profiles, ffProfs...)

	return profiles
}

func parseLocalStateProfiles(path, browser, keyring string) []BrowserProfile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var root struct {
		Profile struct {
			InfoCache map[string]struct {
				Name     string `json:"name"`
				UserName string `json:"user_name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}

	var res []BrowserProfile
	for key, item := range root.Profile.InfoCache {
		displayName := item.Name
		if displayName == "" {
			displayName = key
		}

		spec := fmt.Sprintf("%s%s:%s", browser, keyring, key)
		res = append(res, BrowserProfile{
			Browser:     browser,
			ProfileKey:  key,
			Name:        displayName,
			UserName:    item.UserName,
			BrowserSpec: spec,
		})
	}

	return res
}

func parseFirefoxProfiles(iniPath string) []BrowserProfile {
	f, err := os.Open(iniPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var profiles []BrowserProfile
	scanner := bufio.NewScanner(f)
	var currentName, currentPath string
	inProfileSection := false

	flushProfile := func() {
		if inProfileSection && currentName != "" {
			profileKey := currentPath
			if profileKey == "" {
				profileKey = currentName
			}
			profiles = append(profiles, BrowserProfile{
				Browser:     "firefox",
				ProfileKey:  profileKey,
				Name:        currentName,
				BrowserSpec: "firefox:" + profileKey,
			})
		}
		currentName = ""
		currentPath = ""
		inProfileSection = false
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flushProfile()
			sec := line[1 : len(line)-1]
			if strings.HasPrefix(strings.ToLower(sec), "profile") {
				inProfileSection = true
			}
			continue
		}

		if inProfileSection {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.ToLower(strings.TrimSpace(parts[0]))
				v := strings.TrimSpace(parts[1])
				switch k {
				case "name":
					currentName = v
				case "path":
					currentPath = v
				}
			}
		}
	}
	flushProfile()

	return profiles
}

// CommandRunner defines the signature for executing external processes.
type CommandRunner func(ctx context.Context, name string, args ...string) error

// DefaultCommandRunner executes commands using os/exec.
func DefaultCommandRunner(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GoogleSignInLauncher launches an isolated browser session for YouTube login.
type GoogleSignInLauncher struct {
	Runner      CommandRunner
	BrowserPath string
}

// NewGoogleSignInLauncher creates a launcher with an injectable command runner.
func NewGoogleSignInLauncher(runner CommandRunner) *GoogleSignInLauncher {
	if runner == nil {
		runner = DefaultCommandRunner
	}
	return &GoogleSignInLauncher{Runner: runner}
}

// FindSupportedBrowser searches the PATH for an installed Chrome, Chromium, or Brave browser.
func FindSupportedBrowser() (string, error) {
	candidates := []string{
		"google-chrome-stable",
		"google-chrome",
		"chromium",
		"brave-browser",
		"brave",
	}
	for _, bin := range candidates {
		if path, err := exec.LookPath(bin); err == nil && path != "" {
			return path, nil
		}
	}
	return "", errors.New("no supported browser found (searched google-chrome, chromium, brave)")
}

// Launch runs the browser with an isolated user-data-dir and returns the yt-dlp browser spec.
func (l *GoogleSignInLauncher) Launch(ctx context.Context, dataDir, accountID string) (browserSpec string, userDataDir string, err error) {
	browserBin := l.BrowserPath
	if browserBin == "" {
		found, err := FindSupportedBrowser()
		if err != nil {
			return "", "", err
		}
		browserBin = found
	}

	if accountID == "" {
		accountID = fmt.Sprintf("acc_%d", time.Now().Unix())
	}

	userDataDir = filepath.Join(dataDir, "browsers", accountID)
	if err := os.MkdirAll(userDataDir, 0o700); err != nil {
		return "", "", fmt.Errorf("creating browser user data dir: %w", err)
	}

	args := []string{
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"https://accounts.google.com/ServiceLogin?service=youtube&continue=https://www.youtube.com/",
	}

	if err := l.Runner(ctx, browserBin, args...); err != nil {
		return "", userDataDir, fmt.Errorf("browser launch failed: %w", err)
	}

	// Determine browser identifier for yt-dlp
	lowerBin := strings.ToLower(filepath.Base(browserBin))
	bName := "chrome"
	if strings.Contains(lowerBin, "chromium") {
		bName = "chromium"
	} else if strings.Contains(lowerBin, "brave") {
		bName = "brave"
	}

	defSpec := platform.DefaultBrowserSpec()
	keyring := ""
	if idx := strings.IndexByte(defSpec, '+'); idx != -1 {
		keyring = defSpec[idx:]
	}

	browserSpec = fmt.Sprintf("%s%s:%s/Default", bName, keyring, userDataDir)
	return browserSpec, userDataDir, nil
}
