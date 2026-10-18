package yt_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lighttube/internal/yt"
)

func TestParseAccountFeedJSON(t *testing.T) {
	data, err := os.ReadFile("../../testdata/account_feed.json")
	if err != nil {
		t.Fatalf("reading account_feed.json fixture: %v", err)
	}

	videos, err := yt.ParseAccountFeedJSON(data)
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	v1 := videos[0]
	if v1.ID != "vid001home01" {
		t.Errorf("expected ID vid001home01, got %q", v1.ID)
	}
	if v1.Title != "First Test Video" {
		t.Errorf("expected title First Test Video, got %q", v1.Title)
	}
	if v1.Channel != "Tech Channel" {
		t.Errorf("expected channel Tech Channel, got %q", v1.Channel)
	}
	if v1.Duration != 345 {
		t.Errorf("expected duration 345, got %d", v1.Duration)
	}
	if v1.ViewCount != 12500 {
		t.Errorf("expected view count 12500, got %d", v1.ViewCount)
	}
	if v1.UploadDate != "20261015" {
		t.Errorf("expected upload date 20261015, got %q", v1.UploadDate)
	}
	if v1.PublishedAt.IsZero() {
		t.Errorf("expected non-zero PublishedAt from upload date")
	}
}

func TestParseAccountFeedJSON_NAFields(t *testing.T) {
	data, err := os.ReadFile("../../testdata/account_feed_na.json")
	if err != nil {
		t.Fatalf("reading account_feed_na.json fixture: %v", err)
	}

	videos, err := yt.ParseAccountFeedJSON(data)
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	// In account_feed_na.json, there are 5 entries:
	// - 1 valid entry with view_count="NA"
	// - 1 valid entry with channel="NA", upload_date="NA", url=null
	// - 1 valid entry with duration="NA"
	// - 1 invalid entry with id="NA" (must be skipped)
	// - 1 invalid entry with id="" (must be skipped)
	if len(videos) != 3 {
		t.Fatalf("expected 3 valid videos, got %d", len(videos))
	}

	// Video 1: view_count was "NA", must stay 0
	if videos[0].ViewCount != 0 {
		t.Errorf("expected view_count 0 for NA view count, got %d", videos[0].ViewCount)
	}

	// Video 2: channel was "NA", upload_date was "NA", url was null
	if videos[1].Channel != "" {
		t.Errorf("expected channel empty for NA channel, got %q", videos[1].Channel)
	}
	if videos[1].UploadDate != "" {
		t.Errorf("expected upload_date empty for NA upload date, got %q", videos[1].UploadDate)
	}
	if !strings.HasPrefix(videos[1].URL, "https://www.youtube.com/watch?v=") {
		t.Errorf("expected fallback watch URL, got %q", videos[1].URL)
	}

	// Video 3: duration was "NA", must stay 0
	if videos[2].Duration != 0 {
		t.Errorf("expected duration 0 for NA duration, got %d", videos[2].Duration)
	}
	if videos[2].ViewCount != 9876 {
		t.Errorf("expected view_count 9876, got %d", videos[2].ViewCount)
	}
}

func TestSanitizeErrorOutput(t *testing.T) {
	raw := strings.Join([]string{
		"[debug] Encodings: locale UTF-8, fs utf-8",
		"ERROR: [youtube:history] Login details are needed to download this content. Use --cookies-from-browser or --cookies for the authentication.",
		"Cookie: SID=secret_value_12345; HSID=top_secret_67890",
		"Set-Cookie: SSID=super_secret; Domain=.youtube.com; Path=/; Secure",
		".youtube.com\tTRUE\t/\tTRUE\t1792333424\tSID\tsecret_cookie_data",
		"[Cookies] Loading cookie 10/900",
		"WARNING: [youtube:tab] Unsupported lockup view model",
	}, "\n")

	sanitized := yt.SanitizeErrorOutput(raw)

	if strings.Contains(sanitized, "secret_value_12345") {
		t.Errorf("sanitized error leaked Cookie SID value: %s", sanitized)
	}
	if strings.Contains(sanitized, "super_secret") {
		t.Errorf("sanitized error leaked Set-Cookie value: %s", sanitized)
	}
	if strings.Contains(sanitized, "secret_cookie_data") {
		t.Errorf("sanitized error leaked Netscape cookie record: %s", sanitized)
	}
	if strings.Contains(sanitized, "[Cookies]") {
		t.Errorf("sanitized error leaked [Cookies] log line: %s", sanitized)
	}
	if !strings.Contains(sanitized, "Login details are needed") {
		t.Errorf("sanitized error dropped login instruction: %s", sanitized)
	}
	if !strings.Contains(sanitized, "Unsupported lockup view model") {
		t.Errorf("sanitized error dropped standard warning: %s", sanitized)
	}
}

func createFakeYTDLP(t *testing.T, scriptBody string) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "fake-yt-dlp")
	content := fmt.Sprintf("#!/bin/sh\n%s\n", scriptBody)
	if err := os.WriteFile(binPath, []byte(content), 0o755); err != nil {
		t.Fatalf("writing fake yt-dlp: %v", err)
	}
	return binPath
}

func TestAccount_SignIn_SignOut_SignedIn(t *testing.T) {
	tempDir := t.TempDir()
	cookieFile := filepath.Join(tempDir, "lighttube", "cookies.txt")
	cacheDir := filepath.Join(tempDir, "cache")

	// Fake script that checks file mode and writes dummy cookies.
	script := `
for arg in "$@"; do
    if [ "$prev" = "--cookies" ]; then
        COOKIE_PATH="$arg"
    fi
    prev="$arg"
done
if [ -n "$COOKIE_PATH" ]; then
    printf "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1999999999\tTEST\t1\n" > "$COOKIE_PATH"
fi
exit 0
`
	bin := createFakeYTDLP(t, script)

	account := yt.NewAccountWithOptions(yt.AccountOptions{
		Browser:    "chrome+gnomekeyring",
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: bin,
	})

	if account.SignedIn() {
		t.Fatalf("expected SignedIn to be false initially")
	}

	if err := account.SignIn(context.Background()); err != nil {
		t.Fatalf("SignIn failed: %v", err)
	}

	if !account.SignedIn() {
		t.Fatalf("expected SignedIn to be true after SignIn")
	}

	// Verify file mode is 0600.
	fi, err := os.Stat(cookieFile)
	if err != nil {
		t.Fatalf("stat cookie file: %v", err)
	}
	perm := fi.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected cookie file permission 0600, got %04o", perm)
	}

	// Test YtDlpArgs when signed in.
	args := account.YtDlpArgs()
	expectedArg := "--ytdl-raw-options=cookies=" + cookieFile
	if len(args) != 1 || args[0] != expectedArg {
		t.Errorf("expected YtDlpArgs %q, got %v", expectedArg, args)
	}

	// Test SignOut.
	if err := account.SignOut(); err != nil {
		t.Fatalf("SignOut failed: %v", err)
	}

	if account.SignedIn() {
		t.Fatalf("expected SignedIn to be false after SignOut")
	}
	if argsAfter := account.YtDlpArgs(); argsAfter != nil {
		t.Errorf("expected YtDlpArgs to return nil after SignOut, got %v", argsAfter)
	}
}

func TestAccount_Feed_And_Cached(t *testing.T) {
	tempDir := t.TempDir()
	cookieFile := filepath.Join(tempDir, "cookies.txt")
	cacheDir := filepath.Join(tempDir, "cache")

	fixturePath, err := filepath.Abs("../../testdata/account_feed.json")
	if err != nil {
		t.Fatalf("getting fixture path: %v", err)
	}

	script := fmt.Sprintf(`
# Check if signing in
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

	bin := createFakeYTDLP(t, script)

	account := yt.NewAccountWithOptions(yt.AccountOptions{
		Browser:    "chrome+gnomekeyring",
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: bin,
	})

	// Pre-create cookie file so SignedIn is true.
	if err := os.WriteFile(cookieFile, []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1999999999\tTEST\t1\n"), 0o600); err != nil {
		t.Fatalf("writing cookie file: %v", err)
	}

	// Cached should be empty initially.
	cachedBefore, err := account.Cached(yt.FeedHome)
	if err != nil {
		t.Fatalf("Cached returned error on missing cache: %v", err)
	}
	if len(cachedBefore) != 0 {
		t.Fatalf("expected empty cached before Feed, got %d", len(cachedBefore))
	}

	// Fetch Feed.
	videos, err := account.Feed(context.Background(), yt.FeedHome)
	if err != nil {
		t.Fatalf("Feed failed: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	// Verify disk cache file exists.
	diskCacheFile := filepath.Join(cacheDir, "home.json")
	if _, err := os.Stat(diskCacheFile); err != nil {
		t.Fatalf("expected cache file at %s: %v", diskCacheFile, err)
	}

	// Verify Cached reader.
	cachedAfter, err := account.Cached(yt.FeedHome)
	if err != nil {
		t.Fatalf("Cached failed: %v", err)
	}
	if len(cachedAfter) != 2 {
		t.Fatalf("expected 2 cached videos, got %d", len(cachedAfter))
	}
	if cachedAfter[0].ID != videos[0].ID {
		t.Errorf("cached video ID %q does not match fetched %q", cachedAfter[0].ID, videos[0].ID)
	}
}

func TestAccount_Feed_LoginRetry(t *testing.T) {
	tempDir := t.TempDir()
	cookieFile := filepath.Join(tempDir, "cookies.txt")
	cacheDir := filepath.Join(tempDir, "cache")
	stateFile := filepath.Join(tempDir, "attempts")

	fixturePath, err := filepath.Abs("../../testdata/account_feed.json")
	if err != nil {
		t.Fatalf("getting fixture path: %v", err)
	}

	// Script fails on first feed fetch with login required, then succeeds on retry.
	script := fmt.Sprintf(`
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

if [ ! -f %q ]; then
    touch %q
    echo "ERROR: [youtube:history] Login details are needed to download this content. Use --cookies-from-browser or --cookies for the authentication." >&2
    echo "null"
    exit 0
fi

cat %q
exit 0
`, stateFile, stateFile, fixturePath)

	bin := createFakeYTDLP(t, script)

	account := yt.NewAccountWithOptions(yt.AccountOptions{
		Browser:    "chrome+gnomekeyring",
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: bin,
	})

	if err := os.WriteFile(cookieFile, []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1999999999\tTEST\t1\n"), 0o600); err != nil {
		t.Fatalf("writing cookie file: %v", err)
	}

	// Feed should hit the login error, trigger SignIn once, then retry and succeed.
	videos, err := account.Feed(context.Background(), yt.FeedHistory)
	if err != nil {
		t.Fatalf("expected Feed to succeed on retry after re-sign-in, got: %v", err)
	}
	if len(videos) != 2 {
		t.Errorf("expected 2 videos, got %d", len(videos))
	}
}

func TestAccount_Feed_LoginRequiredFailure(t *testing.T) {
	tempDir := t.TempDir()
	cookieFile := filepath.Join(tempDir, "cookies.txt")
	cacheDir := filepath.Join(tempDir, "cache")

	// Script consistently returns login error.
	script := `
echo "ERROR: [youtube:history] Login details are needed to download this content. Use --cookies-from-browser or --cookies for the authentication." >&2
echo "null"
exit 1
`
	bin := createFakeYTDLP(t, script)

	account := yt.NewAccountWithOptions(yt.AccountOptions{
		Browser:    "chrome+gnomekeyring",
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: bin,
	})

	_, err := account.Feed(context.Background(), yt.FeedHistory)
	if err == nil {
		t.Fatal("expected ErrLoginRequired, got nil")
	}
	if !errors.Is(err, yt.ErrLoginRequired) {
		t.Fatalf("expected errors.Is(err, ErrLoginRequired) to be true, got %v", err)
	}
}
