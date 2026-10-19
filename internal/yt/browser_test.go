package yt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverBrowserProfiles(t *testing.T) {
	homeDir := t.TempDir()

	// 1. Create fake Chrome Local State
	chromeDir := filepath.Join(homeDir, ".config", "google-chrome")
	if err := os.MkdirAll(chromeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chromeLocalState := `{
		"profile": {
			"info_cache": {
				"Default": {
					"name": "Personal",
					"user_name": "personal@example.com"
				},
				"Profile 1": {
					"name": "Work",
					"user_name": "work@example.com"
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(chromeDir, "Local State"), []byte(chromeLocalState), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Create fake Firefox profiles.ini
	firefoxDir := filepath.Join(homeDir, ".mozilla", "firefox")
	if err := os.MkdirAll(firefoxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	firefoxIni := `[General]
StartWithLastProfile=1

[Profile0]
Name=default-release
IsRelative=1
Path=abcd.default-release
Default=1

[Profile1]
Name=dev-edition
IsRelative=1
Path=efgh.dev-edition
`
	if err := os.WriteFile(filepath.Join(firefoxDir, "profiles.ini"), []byte(firefoxIni), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles := DiscoverBrowserProfilesFrom(homeDir)
	if len(profiles) != 4 {
		t.Fatalf("expected 4 profiles (2 chrome, 2 firefox), got %d: %+v", len(profiles), profiles)
	}

	var foundChromePersonal, foundChromeWork, foundFirefoxRelease bool
	for _, p := range profiles {
		if p.Browser == "chrome" && p.ProfileKey == "Default" && p.UserName == "personal@example.com" {
			foundChromePersonal = true
			if !strings.HasPrefix(p.BrowserSpec, "chrome") || !strings.HasSuffix(p.BrowserSpec, ":Default") {
				t.Errorf("unexpected chrome spec: %s", p.BrowserSpec)
			}
		}
		if p.Browser == "chrome" && p.ProfileKey == "Profile 1" && p.UserName == "work@example.com" {
			foundChromeWork = true
		}
		if p.Browser == "firefox" && p.Name == "default-release" && p.BrowserSpec == "firefox:abcd.default-release" {
			foundFirefoxRelease = true
		}
	}

	if !foundChromePersonal {
		t.Error("missing Chrome Personal profile")
	}
	if !foundChromeWork {
		t.Error("missing Chrome Work profile")
	}
	if !foundFirefoxRelease {
		t.Error("missing Firefox default-release profile")
	}
}

func TestGoogleSignInLauncherWithFakeRunner(t *testing.T) {
	dataDir := t.TempDir()

	var commandRun string
	var argsRun []string

	fakeRunner := func(ctx context.Context, name string, args ...string) error {
		commandRun = name
		argsRun = args
		return nil
	}

	launcher := NewGoogleSignInLauncher(fakeRunner)
	launcher.BrowserPath = "/usr/bin/google-chrome"

	spec, userDir, err := launcher.Launch(context.Background(), dataDir, "account123")
	if err != nil {
		t.Fatalf("launcher error: %v", err)
	}

	if commandRun != "/usr/bin/google-chrome" {
		t.Errorf("expected command /usr/bin/google-chrome, got %s", commandRun)
	}

	expectedUserDataDir := filepath.Join(dataDir, "browsers", "account123")
	if userDir != expectedUserDataDir {
		t.Errorf("expected user dir %s, got %s", expectedUserDataDir, userDir)
	}

	expectedArgPrefix := "--user-data-dir=" + expectedUserDataDir
	foundUserDirArg := false
	foundURLArg := false
	for _, a := range argsRun {
		if a == expectedArgPrefix {
			foundUserDirArg = true
		}
		if strings.Contains(a, "accounts.google.com/ServiceLogin") {
			foundURLArg = true
		}
	}

	if !foundUserDirArg {
		t.Errorf("missing user-data-dir argument, args were: %v", argsRun)
	}
	if !foundURLArg {
		t.Errorf("missing login URL argument, args were: %v", argsRun)
	}

	if !strings.HasPrefix(spec, "chrome") || !strings.HasSuffix(spec, expectedUserDataDir+"/Default") {
		t.Errorf("unexpected browser spec: %s", spec)
	}
}
