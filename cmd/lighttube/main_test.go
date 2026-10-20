package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	idemo "lighttube/internal/demo"
	"lighttube/internal/store"
	"lighttube/internal/yt"
)

type mockYTDLP struct {
	searchData []byte
	resolveID  string
}

func (m *mockYTDLP) Search(ctx context.Context, query string) ([]byte, error) {
	return m.searchData, nil
}

func (m *mockYTDLP) ResolveChannel(ctx context.Context, target string) (string, error) {
	if m.resolveID != "" {
		return m.resolveID, nil
	}
	return "UCmock12345678901234567", nil
}

func setupTestEnv(t *testing.T) (*appEnv, *bytes.Buffer, *bytes.Buffer) {
	searchFixture, err := os.ReadFile("../../testdata/search.json")
	if err != nil {
		t.Fatalf("failed reading search fixture: %v", err)
	}

	mock := &mockYTDLP{
		searchData: searchFixture,
		resolveID:  "UC8butISFwT-Wl7EV0hUK0BQ",
	}

	tempDir := t.TempDir()
	st, err := store.NewWithDir(filepath.Join(tempDir, "data"))
	if err != nil {
		t.Fatalf("failed creating test store: %v", err)
	}

	cacheDir := filepath.Join(tempDir, "cache")
	// Pre-populate feed cache for the mock channel.
	feedFixture, err := os.ReadFile("../../testdata/feed.xml")
	if err == nil {
		_ = os.MkdirAll(cacheDir, 0o755)
		_ = os.WriteFile(filepath.Join(cacheDir, "UC8butISFwT-Wl7EV0hUK0BQ.xml"), feedFixture, 0o644)
	}

	ytClient := yt.NewClientWithOptions(mock, nil, cacheDir)

	var stdout, stderr bytes.Buffer
	env := &appEnv{
		ytClient: ytClient,
		store:    st,
		stdout:   &stdout,
		stderr:   &stderr,
		stdin:    strings.NewReader(""),
	}

	return env, &stdout, &stderr
}

func TestRunVersion(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)
	if err := runWithEnv(env, []string{"--version"}); err != nil {
		t.Fatalf("unexpected error for --version: %v", err)
	}
	if !strings.Contains(stdout.String(), "lighttube") {
		t.Errorf("expected version output, got: %s", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)
	if err := runWithEnv(env, []string{"--help"}); err != nil {
		t.Fatalf("unexpected error for --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("expected help output, got: %s", stdout.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	env, _, _ := setupTestEnv(t)
	err := runWithEnv(env, []string{})
	if err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestRunPlayMissingURL(t *testing.T) {
	env, _, _ := setupTestEnv(t)
	err := runWithEnv(env, []string{"play"})
	if err == nil {
		t.Fatal("expected error when play is invoked without URL, got nil")
	}
	if !strings.Contains(err.Error(), "usage: lighttube play <url> [--audio]") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	env, _, _ := setupTestEnv(t)
	err := runWithEnv(env, []string{"foo"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRunSearch(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)
	if err := runWithEnv(env, []string{"search", "golang"}); err != nil {
		t.Fatalf("runWithEnv search failed: %v", err)
	}

	out := stdout.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 search result lines, got %d:\n%s", len(lines), out)
	}

	// Verify id, title, channel, duration format.
	parts := strings.Split(lines[0], "\t")
	if len(parts) != 4 {
		t.Fatalf("expected 4 tab-separated fields, got %d: %q", len(parts), lines[0])
	}
	if parts[0] != "yyUHQIec83I" {
		t.Errorf("expected first video ID yyUHQIec83I, got %q", parts[0])
	}
	if parts[3] != "3:24:59" {
		t.Errorf("expected duration 3:24:59, got %q", parts[3])
	}
}

func TestRunSubAddAndList(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)

	// Add channel.
	if err := runWithEnv(env, []string{"sub", "add", "@freecodecamp"}); err != nil {
		t.Fatalf("sub add failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "added") {
		t.Errorf("expected added message, got: %s", stdout.String())
	}

	// List channels.
	stdout.Reset()
	if err := runWithEnv(env, []string{"sub", "list"}); err != nil {
		t.Fatalf("sub list failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "UC8butISFwT-Wl7EV0hUK0BQ") {
		t.Errorf("expected UC8butISFwT-Wl7EV0hUK0BQ in sub list, got: %s", out)
	}
}

func TestRunFeed(t *testing.T) {
	env, stdout, _ := setupTestEnv(t)

	// Initially empty subscriptions -> empty feed.
	if err := runWithEnv(env, []string{"feed"}); err != nil {
		t.Fatalf("feed on empty subs failed: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty feed output when no subscriptions, got: %s", stdout.String())
	}

	// Add subscription.
	_ = env.store.AddSubscription(yt.Subscription{
		ChannelID: "UC8butISFwT-Wl7EV0hUK0BQ",
		Title:     "freeCodeCamp.org",
	})

	stdout.Reset()
	if err := runWithEnv(env, []string{"feed"}); err != nil {
		t.Fatalf("feed with subscription failed: %v", err)
	}

	out := stdout.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("expected feed lines, got empty output")
	}

	parts := strings.Split(lines[0], "\t")
	if len(parts) != 4 {
		t.Fatalf("expected 4 tab-separated fields in feed output, got %d: %q", len(parts), lines[0])
	}
}

type fileSnap struct {
	size   int64
	mtime  time.Time
	sha256 [32]byte
}

func snapshotTree(t *testing.T, root string) map[string]fileSnap {
	t.Helper()
	snaps := make(map[string]fileSnap)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snaps[rel] = fileSnap{
			size:   info.Size(),
			mtime:  info.ModTime(),
			sha256: sha256.Sum256(data),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotting %s: %v", root, err)
	}
	return snaps
}

func TestDemoIsolationLiveXDGDirsUnchanged(t *testing.T) {
	// (a) Demo never creates or reads a store, account or cookie file under live XDG dirs.
	canaryData := t.TempDir()
	canaryCache := t.TempDir()

	// Put canaries at the real paths:
	// lighttube/settings.json, lighttube/cookies.txt, lighttube/cookies.txt.play, lighttube/accounts/x/cookies.txt
	dataLt := filepath.Join(canaryData, "lighttube")
	if err := os.MkdirAll(filepath.Join(dataLt, "accounts", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataLt, "settings.json"), []byte(`{"active_account":"live_user"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataLt, "cookies.txt"), []byte("canary-secret-cookie-live"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataLt, "cookies.txt.play"), []byte("canary-secret-play-cookie"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataLt, "accounts", "x", "cookies.txt"), []byte("canary-account-cookie"), 0o600); err != nil {
		t.Fatal(err)
	}

	// In canaryCache: lighttube/account/home.json
	cacheLt := filepath.Join(canaryCache, "lighttube")
	if err := os.MkdirAll(filepath.Join(cacheLt, "account"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheLt, "account", "home.json"), []byte(`{"sections":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MESA_SHADER_CACHE_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", canaryData)
	t.Setenv("XDG_CACHE_HOME", canaryCache)

	snapDataBefore := snapshotTree(t, dataLt)
	snapCacheBefore := snapshotTree(t, cacheLt)

	// Chmod the two lighttube dirs to 000 during the run so any open/stat fails.
	if err := os.Chmod(dataLt, 0000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cacheLt, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dataLt, 0o755)
		_ = os.Chmod(cacheLt, 0o755)
	})

	outPNG := filepath.Join(t.TempDir(), "out.png")
	err := run([]string{"--demo", "--render-png", "100x100", outPNG})
	if err != nil {
		t.Fatalf("run with --demo failed: %v", err)
	}

	// Restore permissions before comparing snapshots
	_ = os.Chmod(dataLt, 0o755)
	_ = os.Chmod(cacheLt, 0o755)

	snapDataAfter := snapshotTree(t, dataLt)
	snapCacheAfter := snapshotTree(t, cacheLt)

	// Compare full snapshots before and after
	if len(snapDataAfter) != len(snapDataBefore) {
		t.Fatalf("data directory file count changed: before %d, after %d", len(snapDataBefore), len(snapDataAfter))
	}
	for k, b := range snapDataBefore {
		a, ok := snapDataAfter[k]
		if !ok {
			t.Fatalf("canary file %s missing after demo run", k)
		}
		if a.size != b.size || a.mtime != b.mtime || a.sha256 != b.sha256 {
			t.Fatalf("canary file %s modified by demo run (size %d->%d, hash %x->%x)", k, b.size, a.size, b.sha256, a.sha256)
		}
	}

	if len(snapCacheAfter) != len(snapCacheBefore) {
		t.Fatalf("cache directory file count changed: before %d, after %d", len(snapCacheBefore), len(snapCacheAfter))
	}
	for k, b := range snapCacheBefore {
		a, ok := snapCacheAfter[k]
		if !ok {
			t.Fatalf("canary cache file %s missing after demo run", k)
		}
		if a.size != b.size || a.mtime != b.mtime || a.sha256 != b.sha256 {
			t.Fatalf("canary cache file %s modified by demo run (size %d->%d, hash %x->%x)", k, b.size, a.size, b.sha256, a.sha256)
		}
	}
}

func TestDemoIsolationBuiltBinaryFromEmptyCWD(t *testing.T) {
	// (d) Test with built binary: cd $(mktemp -d) && <abs path>/lighttube --demo --render-png 1440x1080 out.png
	// must succeed with an empty cwd and no repo or scratchpad paths.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "lighttube")

	buildCmd := exec.Command("go", "build", "-tags", "novulkan", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("building binary failed: %v\nOutput: %s", err, string(out))
	}

	emptyCwd := t.TempDir()
	outPNG := filepath.Join(emptyCwd, "out.png")

	runCmd := exec.Command(binPath, "--demo", "--render-png", "1440x1080", outPNG)
	runCmd.Dir = emptyCwd
	runCmd.Env = os.Environ()

	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running built binary in empty cwd failed: %v\nOutput: %s", err, string(out))
	}

	info, err := os.Stat(outPNG)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty output PNG at %s, err: %v", outPNG, err)
	}
}

func TestDemoMissingAssetsFailsWithoutFallback(t *testing.T) {
	// (e) If embedded assets are missing or broken, --demo exits with a clear error
	// that names the asset, and never falls back to the real account or network.
	canaryData := t.TempDir()
	canaryCache := t.TempDir()
	t.Setenv("XDG_DATA_HOME", canaryData)
	t.Setenv("XDG_CACHE_HOME", canaryCache)

	origFS := idemo.AssetFS
	t.Cleanup(func() {
		idemo.AssetFS = origFS
		idemo.ResetCache()
	})

	// 1. Missing file: map has 11 assets, missing city.jpg
	missingMap := make(fstest.MapFS)
	for _, f := range idemo.DemoFiles {
		if f != "city.jpg" {
			data, err := fs.ReadFile(origFS, f)
			if err != nil {
				t.Fatalf("reading embedded file %s: %v", f, err)
			}
			missingMap[f] = &fstest.MapFile{Data: data}
		}
	}
	idemo.AssetFS = missingMap
	idemo.ResetCache()

	outPNG := filepath.Join(t.TempDir(), "out.png")
	err := run([]string{"--demo", "--render-png", "100x100", outPNG})
	if err == nil {
		t.Fatalf("expected error when city.jpg is missing, got nil")
	}
	if !strings.Contains(err.Error(), "city.jpg") {
		t.Fatalf("expected error to name 'city.jpg', got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(canaryData, "lighttube")); !os.IsNotExist(statErr) {
		t.Fatalf("demo mode created lighttube dir under canaryData on missing asset error")
	}

	// 2. Corrupted JPEG: workshop.jpg has invalid bytes
	corruptMap := make(fstest.MapFS)
	for _, f := range idemo.DemoFiles {
		if f == "workshop.jpg" {
			corruptMap[f] = &fstest.MapFile{Data: []byte("not a valid jpeg image")}
		} else {
			data, err := fs.ReadFile(origFS, f)
			if err != nil {
				t.Fatalf("reading embedded file %s: %v", f, err)
			}
			corruptMap[f] = &fstest.MapFile{Data: data}
		}
	}
	idemo.AssetFS = corruptMap
	idemo.ResetCache()

	err = run([]string{"--demo", "--render-png", "100x100", outPNG})
	if err == nil {
		t.Fatalf("expected error when workshop.jpg is corrupted, got nil")
	}
	if !strings.Contains(err.Error(), "workshop.jpg") {
		t.Fatalf("expected error to name 'workshop.jpg', got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(canaryData, "lighttube")); !os.IsNotExist(statErr) {
		t.Fatalf("demo mode created lighttube dir under canaryData on corrupt asset error")
	}
}
