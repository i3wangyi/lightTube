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

	"lighttube/internal/platform"
)

// DefaultBrowserSpec returns the default browser and keyring specification for yt-dlp cookie extraction.
func DefaultBrowserSpec() string {
	return platform.DefaultBrowserSpec()
}

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

// DefaultDataDir returns the base directory under platform DataDir.
func DefaultDataDir() string {
	return platform.DataDir()
}

// DefaultCookiePath returns the default location of the extracted cookies file.
func DefaultCookiePath() string {
	return filepath.Join(DefaultDataDir(), "cookies.txt")
}

// DefaultAccountCacheDir returns the feed cache directory under platform CacheDir.
func DefaultAccountCacheDir() string {
	return filepath.Join(platform.CacheDir(), "account")
}

// Account manages YouTube account authentication and authenticated feeds.
type Account struct {
	mu sync.RWMutex
	// runMu serializes yt-dlp runs: each run rewrites the whole cookie file
	// on exit, so parallel runs could interleave and corrupt it.
	runMu       sync.Mutex
	channelsMu  sync.Mutex
	playlistsMu sync.Mutex
	Browser     string
	CookieFile  string
	CacheDir    string
	BinaryPath  string
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
		browser = DefaultBrowserSpec()
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
	_ = os.Remove(a.CookieFile + ".play")
	return nil
}

// SetBrowser sets the --cookies-from-browser spec for later sign-ins.
func (a *Account) SetBrowser(spec string) {
	a.mu.Lock()
	a.Browser = spec
	a.mu.Unlock()
}

// SignIn extracts cookies once from the specified browser into the cookie file with mode 0600.
// It extracts into a temporary file and renames it, so a failed sign-in keeps the old cookies.
func (a *Account) SignIn(ctx context.Context) error {
	a.mu.RLock()
	browser, cookieFile := a.Browser, a.CookieFile
	a.mu.RUnlock()

	// Hold runMu, not mu, for the run: the UI calls SignedIn on every frame,
	// and the extraction takes about 10 s.
	a.runMu.Lock()
	defer a.runMu.Unlock()

	dir := filepath.Dir(cookieFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating cookie directory: %w", err)
	}

	const header = "# Netscape HTTP Cookie File\n"
	tmp := cookieFile + ".new"
	defer os.Remove(tmp)
	// Create the file with 0600 before yt-dlp writes to it.
	if err := os.WriteFile(tmp, []byte(header), 0o600); err != nil {
		return fmt.Errorf("creating cookie file: %w", err)
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"--cookies-from-browser", browser,
		"--cookies", tmp,
		"--skip-download",
		"--playlist-items", "0",
		"https://www.youtube.com",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	// yt-dlp may recreate the file with the umask mode.
	_ = os.Chmod(tmp, 0o600)

	if runErr != nil {
		cleanStderr := SanitizeErrorOutput(stderr.String())
		if isLoginSignal(cleanStderr) {
			return fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return fmt.Errorf("yt-dlp cookie extraction error: %w (stderr: %s)", runErr, cleanStderr)
	}

	fi, statErr := os.Stat(tmp)
	if statErr != nil || fi.Size() <= int64(len(header)) {
		return errors.New("cookie extraction produced empty cookie file")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.Rename(tmp, cookieFile); err != nil {
		return fmt.Errorf("installing cookie file: %w", err)
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

// Channels fetches the account's subscribed channels list via yt-dlp, caching results to disk.
// At most one channels fetch is in flight at a time.
func (a *Account) Channels(ctx context.Context) ([]Channel, error) {
	if !a.SignedIn() {
		return nil, ErrLoginRequired
	}

	a.channelsMu.Lock()
	defer a.channelsMu.Unlock()

	// For read-only calls like Channels and Storyboard, we pass a copy of the
	// cookie file (created via PlaybackCookieFile) rather than a.CookieFile. Because
	// yt-dlp rewrites whatever cookie file is passed to --cookies on exit, using a copy
	// ensures that these read-only calls do not hold a.runMu while executing network
	// requests, avoiding blocking concurrent or subsequent account feed refreshes.
	cookieCopy, err := a.PlaybackCookieFile()
	if err != nil {
		return nil, fmt.Errorf("preparing playback cookie file: %w", err)
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"--cookies", cookieCopy,
		"--flat-playlist",
		"-J",
		"https://www.youtube.com/feed/channels",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	cleanStderr := SanitizeErrorOutput(stderr.String())
	if runErr != nil {
		if isLoginSignal(cleanStderr) {
			return nil, fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return nil, fmt.Errorf("yt-dlp channels error: %w (stderr: %s)", runErr, cleanStderr)
	}

	channels, err := ParseAccountChannelsJSON(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("parsing account channels: %w", err)
	}

	cacheFile := filepath.Join(a.CacheDir, "channels.json")
	if enc, encErr := json.MarshalIndent(channels, "", "  "); encErr == nil {
		_ = atomicWriteFile(cacheFile, enc)
	}

	return channels, nil
}

// CachedChannels reads the cached subscribed channels list without network execution.
func (a *Account) CachedChannels() ([]Channel, error) {
	cacheFile := filepath.Join(a.CacheDir, "channels.json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cached channels: %w", err)
	}

	var channels []Channel
	if err := json.Unmarshal(data, &channels); err != nil {
		return nil, fmt.Errorf("decoding cached channels: %w", err)
	}

	return channels, nil
}

// Playlists fetches the account's saved playlists list via yt-dlp, caching results to disk.
// At most one playlists fetch is in flight at a time.
func (a *Account) Playlists(ctx context.Context) ([]Playlist, error) {
	if !a.SignedIn() {
		return nil, ErrLoginRequired
	}

	a.playlistsMu.Lock()
	defer a.playlistsMu.Unlock()

	cookieCopy, err := a.PlaybackCookieFile()
	if err != nil {
		return nil, fmt.Errorf("preparing playback cookie file: %w", err)
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"--cookies", cookieCopy,
		"--flat-playlist",
		"-J",
		"https://www.youtube.com/feed/playlists",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	cleanStderr := SanitizeErrorOutput(stderr.String())
	if runErr != nil {
		if isLoginSignal(cleanStderr) {
			return nil, fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return nil, fmt.Errorf("yt-dlp playlists error: %w (stderr: %s)", runErr, cleanStderr)
	}

	playlists, err := ParseAccountPlaylistsJSON(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("parsing account playlists: %w", err)
	}

	cacheFile := filepath.Join(a.CacheDir, "playlists.json")
	if enc, encErr := json.MarshalIndent(playlists, "", "  "); encErr == nil {
		_ = atomicWriteFile(cacheFile, enc)
	}

	return playlists, nil
}

// CachedPlaylists reads the cached saved playlists list without network execution.
func (a *Account) CachedPlaylists() ([]Playlist, error) {
	cacheFile := filepath.Join(a.CacheDir, "playlists.json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cached playlists: %w", err)
	}

	var playlists []Playlist
	if err := json.Unmarshal(data, &playlists); err != nil {
		return nil, fmt.Errorf("decoding cached playlists: %w", err)
	}

	return playlists, nil
}

// PlaylistVideos fetches entries for a playlist URL via yt-dlp.
func (a *Account) PlaylistVideos(ctx context.Context, playlistURL string) (string, []Video, error) {
	if playlistURL == "" {
		return "", nil, errors.New("empty playlist URL")
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"--flat-playlist",
		"-J",
		playlistURL,
	}

	if a.SignedIn() {
		if cookieCopy, err := a.PlaybackCookieFile(); err == nil {
			args = append([]string{"--cookies", cookieCopy}, args...)
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	cleanStderr := SanitizeErrorOutput(stderr.String())
	if runErr != nil {
		if isLoginSignal(cleanStderr) {
			return "", nil, fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return "", nil, fmt.Errorf("yt-dlp playlist videos error: %w (stderr: %s)", runErr, cleanStderr)
	}

	return ParsePlaylistDetailJSON(stdout.Bytes())
}

// Storyboard fetches video storyboard format (prefers sb0) via yt-dlp, caching to disk for 24 hours.
func (a *Account) Storyboard(ctx context.Context, videoID string) (*Storyboard, error) {
	if videoID == "" {
		return nil, errors.New("empty video ID")
	}

	cacheDir := filepath.Join(a.CacheDir, "storyboards")
	cacheFile := filepath.Join(cacheDir, videoID+".json")

	if fi, err := os.Stat(cacheFile); err == nil && time.Since(fi.ModTime()) < 24*time.Hour {
		if data, readErr := os.ReadFile(cacheFile); readErr == nil {
			var sb Storyboard
			if decErr := json.Unmarshal(data, &sb); decErr == nil && len(sb.Sheets) > 0 {
				return &sb, nil
			}
		}
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{
		"-J",
		"https://www.youtube.com/watch?v=" + videoID,
	}

	if a.SignedIn() {
		// For read-only calls like Channels and Storyboard, we pass a copy of the
		// cookie file (created via PlaybackCookieFile) rather than a.CookieFile. Because
		// yt-dlp rewrites whatever cookie file is passed to --cookies on exit, using a copy
		// ensures that these read-only calls do not hold a.runMu while executing network
		// requests, avoiding blocking concurrent or subsequent account feed refreshes.
		if cookieCopy, err := a.PlaybackCookieFile(); err == nil {
			args = append([]string{"--cookies", cookieCopy}, args...)
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		cleanStderr := SanitizeErrorOutput(stderr.String())
		return nil, fmt.Errorf("yt-dlp storyboard error: %w (stderr: %s)", runErr, cleanStderr)
	}

	sb, err := ParseStoryboardJSON(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("parsing storyboard JSON: %w", err)
	}
	if sb == nil {
		return nil, nil
	}

	if enc, encErr := json.MarshalIndent(sb, "", "  "); encErr == nil {
		_ = atomicWriteFile(cacheFile, enc)
	}

	return sb, nil
}

// CachedStoryboard reads the cached storyboard list from disk without executing network calls.
func (a *Account) CachedStoryboard(videoID string) (*Storyboard, error) {
	if videoID == "" {
		return nil, errors.New("empty video ID")
	}
	cacheFile := filepath.Join(a.CacheDir, "storyboards", videoID+".json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cached storyboard: %w", err)
	}

	var sb Storyboard
	if err := json.Unmarshal(data, &sb); err != nil {
		return nil, fmt.Errorf("decoding cached storyboard: %w", err)
	}

	return &sb, nil
}

// PlaybackCookieFile copies the cookie file for one mpv playback and returns
// the copy's path. mpv's yt-dlp writes the cookie jar back on exit; with its
// own copy it cannot interleave with a feed refresh that runs at that time.
func (a *Account) PlaybackCookieFile() (string, error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	a.mu.RLock()
	data, err := os.ReadFile(a.CookieFile)
	a.mu.RUnlock()
	if err != nil {
		return "", err
	}
	dst := a.CookieFile + ".play"
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return "", err
	}
	return dst, nil
}

// YtDlpArgs returns extra arguments for mpv player to use cookies if signed in.
func (a *Account) YtDlpArgs() []string {
	if !a.SignedIn() {
		return nil
	}
	path, err := a.PlaybackCookieFile()
	if err != nil {
		return nil
	}
	return []string{"--ytdl-raw-options=cookies=" + path}
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

// ParseAccountChannelsJSON parses yt-dlp flat-playlist -J output for account channels.
func ParseAccountChannelsJSON(data []byte) ([]Channel, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return []Channel{}, nil
	}

	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decoding channels JSON: %w", err)
	}

	channels := make([]Channel, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		id := cleanString(entry["id"])
		if id == "" {
			id = cleanString(entry["channel_id"])
		}
		if id == "" {
			continue
		}

		title := cleanString(entry["title"])
		if title == "" {
			title = cleanString(entry["channel"])
		}
		if title == "" {
			title = cleanString(entry["uploader"])
		}

		followerCount := cleanInt64(entry["channel_follower_count"])

		channelURL := cleanString(entry["url"])
		if channelURL == "" {
			channelURL = cleanString(entry["channel_url"])
		}
		if channelURL == "" {
			channelURL = "https://www.youtube.com/channel/" + id
		}

		var avatarURL string
		if rawThumbs, ok := entry["thumbnails"].([]any); ok && len(rawThumbs) > 0 {
			var bestH int
			for _, item := range rawThumbs {
				if m, ok := item.(map[string]any); ok {
					u := cleanString(m["url"])
					if u == "" {
						continue
					}
					h := cleanInt(m["height"])
					if avatarURL == "" || h > bestH {
						avatarURL = u
						bestH = h
					}
				}
			}
		}
		if avatarURL != "" && strings.HasPrefix(avatarURL, "//") {
			avatarURL = "https:" + avatarURL
		}

		channels = append(channels, Channel{
			ID:            id,
			Title:         title,
			AvatarURL:     avatarURL,
			FollowerCount: followerCount,
			URL:           channelURL,
		})
	}

	return channels, nil
}

// ParseAccountPlaylistsJSON parses yt-dlp flat-playlist -J output for account playlists.
func ParseAccountPlaylistsJSON(data []byte) ([]Playlist, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return []Playlist{}, nil
	}

	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decoding playlists JSON: %w", err)
	}

	playlists := make([]Playlist, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		id := cleanString(entry["id"])
		if id == "" {
			continue
		}

		title := cleanString(entry["title"])
		url := cleanString(entry["url"])
		if url == "" {
			url = "https://www.youtube.com/playlist?list=" + id
		}

		channel := cleanString(entry["channel"])
		if channel == "" {
			channel = cleanString(entry["uploader"])
		}
		// YouTube puts its "View full playlist" link label in the owner
		// field of the user's own playlists; it is not a channel name.
		if strings.EqualFold(channel, "View full playlist") {
			channel = ""
		}

		videoCount := cleanInt(entry["playlist_count"])
		if videoCount == 0 {
			videoCount = cleanInt(entry["video_count"])
		}

		var thumbURL string
		if rawThumbs, ok := entry["thumbnails"].([]any); ok && len(rawThumbs) > 0 {
			var bestH int
			for _, item := range rawThumbs {
				if m, ok := item.(map[string]any); ok {
					u := cleanString(m["url"])
					if u == "" {
						continue
					}
					h := cleanInt(m["height"])
					if thumbURL == "" || h > bestH {
						thumbURL = u
						bestH = h
					}
				}
			}
		}
		if thumbURL != "" && strings.HasPrefix(thumbURL, "//") {
			thumbURL = "https:" + thumbURL
		}

		playlists = append(playlists, Playlist{
			ID:           id,
			Title:        title,
			ThumbnailURL: thumbURL,
			VideoCount:   videoCount,
			URL:          url,
			Channel:      channel,
		})
	}

	return playlists, nil
}

// ParsePlaylistDetailJSON parses yt-dlp flat-playlist -J output for a single playlist's entries.
func ParsePlaylistDetailJSON(data []byte) (string, []Video, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return "", []Video{}, nil
	}

	var payload struct {
		Title   string           `json:"title"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", nil, fmt.Errorf("decoding playlist detail JSON: %w", err)
	}

	videos, err := ParseAccountFeedJSON(data)
	if err != nil {
		return payload.Title, nil, err
	}

	return payload.Title, videos, nil
}

// ParseStoryboardJSON parses yt-dlp -J output for storyboard format (prefers sb0, then any sb format).
func ParseStoryboardJSON(data []byte) (*Storyboard, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}

	var payload struct {
		ID      string           `json:"id"`
		Formats []map[string]any `json:"formats"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decoding video formats JSON: %w", err)
	}

	var bestFormat map[string]any
	for _, f := range payload.Formats {
		fid := cleanString(f["format_id"])
		if fid == "sb0" {
			bestFormat = f
			break
		}
	}
	if bestFormat == nil {
		for _, f := range payload.Formats {
			fid := cleanString(f["format_id"])
			note := cleanString(f["format_note"])
			if strings.HasPrefix(fid, "sb") || note == "storyboard" {
				bestFormat = f
				break
			}
		}
	}

	if bestFormat == nil {
		return nil, nil
	}

	formatID := cleanString(bestFormat["format_id"])
	rows := cleanInt(bestFormat["rows"])
	columns := cleanInt(bestFormat["columns"])
	width := cleanInt(bestFormat["width"])
	height := cleanInt(bestFormat["height"])

	var sheets []string
	if frags, ok := bestFormat["fragments"].([]any); ok && len(frags) > 0 {
		for _, item := range frags {
			if m, ok := item.(map[string]any); ok {
				u := cleanString(m["url"])
				if u != "" {
					sheets = append(sheets, u)
				}
			}
		}
	}

	if len(sheets) == 0 {
		directURL := cleanString(bestFormat["url"])
		if directURL != "" && !strings.Contains(directURL, "$M") {
			sheets = append(sheets, directURL)
		}
	}

	if len(sheets) == 0 {
		return nil, nil
	}

	return &Storyboard{
		VideoID:  payload.ID,
		FormatID: formatID,
		Rows:     rows,
		Columns:  columns,
		Width:    width,
		Height:   height,
		Sheets:   sheets,
	}, nil
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
