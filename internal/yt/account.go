package yt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBrowserSpec is the default browser and keyring specification for yt-dlp cookie extraction.
	DefaultBrowserSpec = "chrome+gnomekeyring"
)

// ErrLoginRequired indicates YouTube authentication is missing, expired, or invalid.
var ErrLoginRequired = errors.New("login required")

// FeedKind represents an account feed type.
type FeedKind string

const (
	FeedHome       FeedKind = "home"
	FeedSubs       FeedKind = "subs"
	FeedWatchLater FeedKind = "later"
	FeedHistory    FeedKind = "history"
)

// NormalizeFeedKind normalizes feed kind strings and aliases to FeedKind.
func NormalizeFeedKind(raw string) (FeedKind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "home", "rec", ":ytrec":
		return FeedHome, nil
	case "subs", "subscriptions", ":ytsubs":
		return FeedSubs, nil
	case "later", "watchlater", ":ytwatchlater":
		return FeedWatchLater, nil
	case "history", ":ythistory":
		return FeedHistory, nil
	default:
		return "", fmt.Errorf("unknown account feed kind: %q", raw)
	}
}

// Target returns the yt-dlp special URL for the feed kind.
func (k FeedKind) Target() (string, error) {
	switch k {
	case FeedHome:
		return ":ytrec", nil
	case FeedSubs:
		return ":ytsubs", nil
	case FeedWatchLater, "watchlater":
		return ":ytwatchlater", nil
	case FeedHistory:
		return ":ythistory", nil
	default:
		return "", fmt.Errorf("unknown account feed kind: %q", k)
	}
}

// DefaultDataDir returns the base directory under XDG_DATA_HOME.
func DefaultDataDir() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "lighttube")
}

// DefaultCookiePath returns the default location of the extracted cookies file.
func DefaultCookiePath() string {
	return filepath.Join(DefaultDataDir(), "cookies.txt")
}

// DefaultAccountCacheDir returns the feed cache directory under XDG_CACHE_HOME.
func DefaultAccountCacheDir() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "lighttube", "account")
}

// Account manages YouTube account authentication and authenticated feeds.
type Account struct {
	mu sync.RWMutex
	// runMu serializes yt-dlp runs: each run rewrites the whole cookie file
	// on exit, so parallel runs could interleave and corrupt it.
	runMu      sync.Mutex
	Browser    string
	CookieFile string
	CacheDir   string
	BinaryPath string
}

// AccountOptions configures an Account instance.
type AccountOptions struct {
	Browser    string
	CookieFile string
	CacheDir   string
	BinaryPath string
}

// NewAccount creates an Account with standard system defaults.
func NewAccount() *Account {
	return NewAccountWithOptions(AccountOptions{})
}

// NewAccountWithOptions creates an Account with custom options (used for testing and configuration).
func NewAccountWithOptions(opts AccountOptions) *Account {
	browser := opts.Browser
	if browser == "" {
		browser = DefaultBrowserSpec
	}
	cookieFile := opts.CookieFile
	if cookieFile == "" {
		cookieFile = DefaultCookiePath()
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = DefaultAccountCacheDir()
	}
	bin := opts.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}
	return &Account{
		Browser:    browser,
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: bin,
	}
}

// SignedIn reports whether the cookie file exists and is non-empty.
func (a *Account) SignedIn() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	fi, err := os.Stat(a.CookieFile)
	if err != nil {
		return false
	}
	return !fi.IsDir() && fi.Size() > 0
}

// SignOut removes the cookie file.
func (a *Account) SignOut() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.Remove(a.CookieFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing cookie file: %w", err)
	}
	return nil
}

// SignIn extracts cookies once from the specified browser into the cookie file with mode 0600.
func (a *Account) SignIn(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dir := filepath.Dir(a.CookieFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating cookie directory: %w", err)
	}

	// Create file with 0600 before yt-dlp writes to prevent permission exposure.
	f, err := os.OpenFile(a.CookieFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating cookie file: %w", err)
	}
	if _, err := f.WriteString("# Netscape HTTP Cookie File\n"); err != nil {
		_ = f.Close()
		return fmt.Errorf("initializing cookie file: %w", err)
	}
	_ = f.Close()
	_ = os.Chmod(a.CookieFile, 0o600)

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"--cookies-from-browser", a.Browser,
		"--cookies", a.CookieFile,
		"--skip-download",
		"--playlist-items", "0",
		"https://www.youtube.com",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	a.runMu.Lock()
	runErr := cmd.Run()
	a.runMu.Unlock()
	// Always restore 0600 permissions after external process writes.
	_ = os.Chmod(a.CookieFile, 0o600)

	if runErr != nil {
		cleanStderr := SanitizeErrorOutput(stderr.String())
		if isLoginSignal(cleanStderr) {
			return fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return fmt.Errorf("yt-dlp cookie extraction error: %w (stderr: %s)", runErr, cleanStderr)
	}

	fi, statErr := os.Stat(a.CookieFile)
	if statErr != nil || fi.Size() <= int64(len("# Netscape HTTP Cookie File\n")) {
		return errors.New("cookie extraction produced empty cookie file")
	}

	return nil
}

// Feed fetches an account feed (home, subs, later, history) via yt-dlp, caching results to disk.
// If a login-required error occurs, SignIn is retried once.
func (a *Account) Feed(ctx context.Context, kind FeedKind) ([]Video, error) {
	normKind, err := NormalizeFeedKind(string(kind))
	if err != nil {
		return nil, err
	}
	target, err := normKind.Target()
	if err != nil {
		return nil, err
	}

	if !a.SignedIn() {
		if err := a.SignIn(ctx); err != nil {
			return nil, err
		}
	}

	data, err := a.execFeed(ctx, target)
	if err != nil && isLoginError(err, data) {
		// Re-extract cookies once and retry feed once.
		if signInErr := a.SignIn(ctx); signInErr != nil {
			return nil, ErrLoginRequired
		}
		data, err = a.execFeed(ctx, target)
		if err != nil && isLoginError(err, data) {
			return nil, ErrLoginRequired
		}
	}
	if err != nil {
		return nil, err
	}

	videos, err := ParseAccountFeedJSON(data)
	if err != nil {
		return nil, fmt.Errorf("parsing account feed %s: %w", normKind, err)
	}

	// Persist to disk cache.
	cacheFile := filepath.Join(a.CacheDir, string(normKind)+".json")
	if enc, encErr := json.MarshalIndent(videos, "", "  "); encErr == nil {
		_ = atomicWriteFile(cacheFile, enc)
	}

	return videos, nil
}

func (a *Account) execFeed(ctx context.Context, target string) ([]byte, error) {
	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"--cookies", a.CookieFile,
		"--flat-playlist",
		"-J",
		"--playlist-end", "60",
		target,
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	a.runMu.Lock()
	err := cmd.Run()
	a.runMu.Unlock()
	cleanStderr := SanitizeErrorOutput(stderr.String())
	if err != nil {
		if isLoginSignal(cleanStderr) {
			return stdout.Bytes(), fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return stdout.Bytes(), fmt.Errorf("yt-dlp feed error: %w (stderr: %s)", err, cleanStderr)
	}

	if strings.TrimSpace(stdout.String()) == "null" || (len(cleanStderr) > 0 && isLoginSignal(cleanStderr)) {
		return stdout.Bytes(), fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
	}

	return stdout.Bytes(), nil
}

// Cached reads the cached video list for a feed kind without executing network calls.
func (a *Account) Cached(kind FeedKind) ([]Video, error) {
	normKind, err := NormalizeFeedKind(string(kind))
	if err != nil {
		return nil, err
	}

	cacheFile := filepath.Join(a.CacheDir, string(normKind)+".json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cached feed %s: %w", normKind, err)
	}

	var videos []Video
	if err := json.Unmarshal(data, &videos); err != nil {
		return nil, fmt.Errorf("decoding cached feed %s: %w", normKind, err)
	}

	return videos, nil
}

// YtDlpArgs returns extra arguments for mpv player to use cookies if signed in.
func (a *Account) YtDlpArgs() []string {
	if !a.SignedIn() {
		return nil
	}
	return []string{"--ytdl-raw-options=cookies=" + a.CookieFile}
}

// YtDlpArgs returns extra arguments for mpv player from the default account if signed in.
func YtDlpArgs() []string {
	return NewAccount().YtDlpArgs()
}

// SanitizeErrorOutput strips cookie values and Netscape cookie rows from process output.
func SanitizeErrorOutput(s string) string {
	var cleanLines []string
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Strip Netscape format cookie records (lines with 5+ tabs).
		if strings.Count(line, "\t") >= 5 {
			continue
		}

		lower := strings.ToLower(trimmed)
		// Check for lines referencing cookies.
		if strings.Contains(lower, "cookie") {
			// Retain generic help/flag instructions.
			if strings.Contains(lower, "--cookies") {
				cleanLines = append(cleanLines, line)
				continue
			}
			// Strip any line containing cookie values or headers.
			continue
		}
		cleanLines = append(cleanLines, line)
	}
	return strings.TrimSpace(strings.Join(cleanLines, "\n"))
}

func isLoginSignal(s string) bool {
	lower := strings.ToLower(s)
	signals := []string{
		"login details are needed",
		"use --cookies",
		"login required",
		"sign in",
		"playlist does not exist",
		"confirm you are not a robot",
		"requires authentication",
	}
	for _, sig := range signals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

func isLoginError(err error, stdout []byte) bool {
	if errors.Is(err, ErrLoginRequired) {
		return true
	}
	if err != nil && isLoginSignal(err.Error()) {
		return true
	}
	// yt-dlp outputs 'null' on stdout when playlist does not exist / login is needed.
	if strings.TrimSpace(string(stdout)) == "null" {
		return true
	}
	return false
}

type accountFeedPayload struct {
	Entries []map[string]any `json:"entries"`
}

// ParseAccountFeedJSON parses yt-dlp flat-playlist -J output, mapping entries to Video.
// Unknown fields stay zero; NA values do not become text.
func ParseAccountFeedJSON(data []byte) ([]Video, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return []Video{}, nil
	}

	var payload accountFeedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decoding flat playlist JSON: %w", err)
	}

	videos := make([]Video, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		id := cleanString(entry["id"])
		if id == "" {
			continue
		}

		title := cleanString(entry["title"])

		channel := cleanString(entry["channel"])
		if channel == "" {
			channel = cleanString(entry["uploader"])
		}

		channelID := cleanString(entry["channel_id"])
		if channelID == "" {
			channelID = cleanString(entry["uploader_id"])
		}

		durationSec := cleanInt(entry["duration"])
		viewCount := cleanInt64(entry["view_count"])
		uploadDate := cleanString(entry["upload_date"])

		videoURL := cleanString(entry["url"])
		if videoURL == "" {
			videoURL = "https://www.youtube.com/watch?v=" + id
		}

		var publishedAt time.Time
		if ts := cleanInt64(entry["timestamp"]); ts > 0 {
			publishedAt = time.Unix(ts, 0)
		} else if len(uploadDate) == 8 {
			if t, err := time.Parse("20060102", uploadDate); err == nil {
				publishedAt = t
			}
		}

		videos = append(videos, Video{
			ID:          id,
			Title:       title,
			Channel:     channel,
			ChannelID:   channelID,
			Duration:    durationSec,
			ViewCount:   viewCount,
			UploadDate:  uploadDate,
			PublishedAt: publishedAt,
			URL:         videoURL,
		})
	}

	return videos, nil
}

func cleanString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "na") || strings.EqualFold(s, "none") || strings.EqualFold(s, "null") {
		return ""
	}
	return s
}

func cleanInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0
		}
		return int(n)
	case int:
		if n < 0 {
			return 0
		}
		return n
	case int64:
		if n < 0 {
			return 0
		}
		return int(n)
	case json.Number:
		if f, err := n.Float64(); err == nil && f > 0 {
			return int(f)
		}
		return 0
	case string:
		s := strings.TrimSpace(n)
		if s == "" || strings.EqualFold(s, "na") || strings.EqualFold(s, "none") || strings.EqualFold(s, "null") {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

func cleanInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0
		}
		return int64(n)
	case int:
		if n < 0 {
			return 0
		}
		return int64(n)
	case int64:
		if n < 0 {
			return 0
		}
		return n
	case json.Number:
		if i, err := n.Int64(); err == nil && i > 0 {
			return i
		}
		if f, err := n.Float64(); err == nil && f > 0 {
			return int64(f)
		}
		return 0
	case string:
		s := strings.TrimSpace(n)
		if s == "" || strings.EqualFold(s, "na") || strings.EqualFold(s, "none") || strings.EqualFold(s, "null") {
			return 0
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil && i > 0 {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			return int64(f)
		}
		return 0
	default:
		return 0
	}
}
