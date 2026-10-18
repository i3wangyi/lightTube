package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

func createFakeCLIYTDLP(t *testing.T) string {
	t.Helper()
	fixturePath, err := filepath.Abs("../../testdata/account_feed.json")
	if err != nil {
		t.Fatalf("getting fixture path: %v", err)
	}

	dir := t.TempDir()
	binPath := filepath.Join(dir, "fake-cli-ytdlp")
	content := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do
    if [ "$prev" = "--cookies" ]; then
        COOKIE_PATH="$arg"
    fi
    prev="$arg"
done

for arg in "$@"; do
    if [ "$arg" = "--cookies-from-browser" ]; then
        printf "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1999999999\tTEST\t1\n" > "$COOKIE_PATH"
        exit 0
    fi
done

cat %q
exit 0
`, fixturePath)

	if err := os.WriteFile(binPath, []byte(content), 0o755); err != nil {
		t.Fatalf("writing fake cli yt-dlp: %v", err)
	}
	return binPath
}

func setupAccountTestEnv(t *testing.T) (*appEnv, *bytes.Buffer, *bytes.Buffer) {
	tempDir := t.TempDir()
	st, err := store.NewWithDir(filepath.Join(tempDir, "data"))
	if err != nil {
		t.Fatalf("failed creating store: %v", err)
	}

	bin := createFakeCLIYTDLP(t)

	account := yt.NewAccountWithOptions(yt.AccountOptions{
		Browser:    "chrome+gnomekeyring",
		CookieFile: filepath.Join(tempDir, "data", "cookies.txt"),
		CacheDir:   filepath.Join(tempDir, "cache"),
		BinaryPath: bin,
	})

	var stdout, stderr bytes.Buffer
	env := &appEnv{
		ytClient: yt.NewClient(),
		store:    st,
		account:  account,
		stdout:   &stdout,
		stderr:   &stderr,
		stdin:    strings.NewReader(""),
	}

	return env, &stdout, &stderr
}

func TestCLIAccount_Status_SignIn_SignOut(t *testing.T) {
	env, stdout, _ := setupAccountTestEnv(t)

	// 1. Initial status should be "signed out".
	if err := runWithEnv(env, []string{"account", "status"}); err != nil {
		t.Fatalf("account status failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "signed out" {
		t.Errorf("expected 'signed out', got %q", stdout.String())
	}

	// 2. Sign in.
	stdout.Reset()
	if err := runWithEnv(env, []string{"account", "signin"}); err != nil {
		t.Fatalf("account signin failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "signed in" {
		t.Errorf("expected 'signed in', got %q", stdout.String())
	}

	// 3. Status should now be "signed in".
	stdout.Reset()
	if err := runWithEnv(env, []string{"account", "status"}); err != nil {
		t.Fatalf("account status failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "signed in" {
		t.Errorf("expected 'signed in', got %q", stdout.String())
	}

	// 4. Sign out.
	stdout.Reset()
	if err := runWithEnv(env, []string{"account", "signout"}); err != nil {
		t.Fatalf("account signout failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "signed out" {
		t.Errorf("expected 'signed out', got %q", stdout.String())
	}

	// 5. Status should be "signed out" again.
	stdout.Reset()
	if err := runWithEnv(env, []string{"account", "status"}); err != nil {
		t.Fatalf("account status failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "signed out" {
		t.Errorf("expected 'signed out', got %q", stdout.String())
	}
}

func TestCLIAccount_UnknownCommand(t *testing.T) {
	env, _, _ := setupAccountTestEnv(t)
	err := runWithEnv(env, []string{"account", "foobar"})
	if err == nil {
		t.Fatal("expected error for unknown account subcommand, got nil")
	}
	if !strings.Contains(err.Error(), "unknown account command") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCLIFeed_Account(t *testing.T) {
	kinds := []string{"home", "subs", "later", "history"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			env, stdout, _ := setupAccountTestEnv(t)

			// Execute signin first so cookies exist.
			if err := runWithEnv(env, []string{"account", "signin"}); err != nil {
				t.Fatalf("account signin failed: %v", err)
			}
			stdout.Reset()

			// Run feed --account <kind>
			if err := runWithEnv(env, []string{"feed", "--account", kind}); err != nil {
				t.Fatalf("feed --account %s failed: %v", kind, err)
			}

			out := strings.TrimSpace(stdout.String())
			lines := strings.Split(out, "\n")
			if len(lines) != 2 {
				t.Fatalf("expected 2 output lines for %s, got %d:\n%s", kind, len(lines), out)
			}

			// Format must be: id\ttitle\tchannel
			parts := strings.Split(lines[0], "\t")
			if len(parts) != 3 {
				t.Fatalf("expected 3 tab-separated fields (id, title, channel), got %d: %q", len(parts), lines[0])
			}
			if parts[0] != "vid001home01" {
				t.Errorf("expected ID vid001home01, got %q", parts[0])
			}
			if parts[1] != "First Test Video" {
				t.Errorf("expected title 'First Test Video', got %q", parts[1])
			}
			if parts[2] != "Tech Channel" {
				t.Errorf("expected channel 'Tech Channel', got %q", parts[2])
			}
		})
	}
}

func TestCLIFeed_AccountInvalidKind(t *testing.T) {
	env, _, _ := setupAccountTestEnv(t)
	err := runWithEnv(env, []string{"feed", "--account", "invalid_kind"})
	if err == nil {
		t.Fatal("expected error on invalid feed kind, got nil")
	}
	if !strings.Contains(err.Error(), "unknown account feed kind") {
		t.Errorf("unexpected error: %v", err)
	}
}
