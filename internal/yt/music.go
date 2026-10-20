package yt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Album represents a music album or collection.
type Album struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
	Year         string  `json:"year,omitempty"`
	Tracks       []Video `json:"tracks,omitempty"`
}

// ParseLikedMusicJSON parses the yt-dlp output for https://www.youtube.com/playlist?list=LM.
func ParseLikedMusicJSON(data []byte) ([]Video, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []Video{}, nil
	}

	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("decoding liked music JSON: %w", err)
	}

	if len(payload.Entries) == 0 {
		return []Video{}, nil
	}

	tracks := make([]Video, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		id := strings.TrimSpace(cleanString(entry["id"]))
		if id == "" {
			continue
		}

		title := strings.TrimSpace(cleanString(entry["title"]))
		artist := strings.TrimSpace(cleanString(entry["channel"]))
		if artist == "" {
			artist = strings.TrimSpace(cleanString(entry["uploader"]))
		}

		durationSec := cleanInt(entry["duration"])
		viewCount := cleanInt64(entry["view_count"])

		videoURL := strings.TrimSpace(cleanString(entry["url"]))
		if videoURL == "" {
			videoURL = "https://www.youtube.com/watch?v=" + id
		}

		thumbURL := cleanString(entry["thumbnail"])
		if thumbURL == "" {
			if thumbs, ok := entry["thumbnails"].([]any); ok {
				var bestH int
				for _, t := range thumbs {
					if m, ok := t.(map[string]any); ok {
						u := cleanString(m["url"])
						h := cleanInt(m["height"])
						if u != "" && (thumbURL == "" || h > bestH) {
							thumbURL = u
							bestH = h
						}
					}
				}
			}
		}
		if thumbURL == "" && id != "" {
			thumbURL = fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", id)
		}

		tracks = append(tracks, Video{
			ID:           id,
			Title:        title,
			Channel:      artist,
			Duration:     durationSec,
			ViewCount:    viewCount,
			URL:          videoURL,
			ThumbnailURL: thumbURL,
		})
	}

	return tracks, nil
}

// ParseMusicAlbumJSON parses the yt-dlp flat playlist output for an album page.
// Note: yt-dlp redirects YouTube Music album URLs to the underlying YouTube playlist.
func ParseMusicAlbumJSON(data []byte) (*Album, []Video, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return &Album{}, []Video{}, nil
	}

	var payload struct {
		ID         string           `json:"id"`
		Title      string           `json:"title"`
		Channel    string           `json:"channel"`
		Uploader   string           `json:"uploader"`
		Thumbnails []map[string]any `json:"thumbnails"`
		Entries    []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, nil, fmt.Errorf("decoding music album JSON: %w", err)
	}

	artist := strings.TrimSpace(cleanString(payload.Channel))
	if artist == "" {
		artist = strings.TrimSpace(cleanString(payload.Uploader))
	}

	// Clean "Album - " prefix if present from YouTube playlist title
	albumTitle := strings.TrimSpace(payload.Title)
	albumTitle = strings.TrimPrefix(albumTitle, "Album - ")

	thumbURL := ""
	var maxH int
	for _, item := range payload.Thumbnails {
		u := cleanString(item["url"])
		h := cleanInt(item["height"])
		if u != "" && (thumbURL == "" || h > maxH) {
			thumbURL = u
			maxH = h
		}
	}

	tracks := make([]Video, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		id := strings.TrimSpace(cleanString(entry["id"]))
		if id == "" {
			continue
		}

		title := strings.TrimSpace(cleanString(entry["title"]))
		trackArtist := strings.TrimSpace(cleanString(entry["channel"]))
		if trackArtist == "" {
			trackArtist = strings.TrimSpace(cleanString(entry["uploader"]))
		}
		if trackArtist == "" {
			trackArtist = artist
		}

		durationSec := cleanInt(entry["duration"])
		viewCount := cleanInt64(entry["view_count"])

		videoURL := strings.TrimSpace(cleanString(entry["url"]))
		if videoURL == "" {
			videoURL = "https://www.youtube.com/watch?v=" + id
		}

		thumbURL := cleanString(entry["thumbnail"])
		if thumbURL == "" && id != "" {
			thumbURL = fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", id)
		}

		tracks = append(tracks, Video{
			ID:           id,
			Title:        title,
			Channel:      trackArtist,
			Duration:     durationSec,
			ViewCount:    viewCount,
			URL:          videoURL,
			Description:  albumTitle, // Store collection / album name in Description for display
			ThumbnailURL: thumbURL,
		})
	}

	album := &Album{
		ID:           payload.ID,
		Title:        albumTitle,
		Artist:       artist,
		ThumbnailURL: thumbURL,
		Tracks:       tracks,
	}

	return album, tracks, nil
}

// LikedMusic retrieves the user's Liked Music playlist (list=LM), cached for 1 hour.
func (a *Account) LikedMusic(ctx context.Context) ([]Video, error) {
	cacheDir := filepath.Join(a.CacheDir, "music")
	cacheFile := filepath.Join(cacheDir, "liked.json")

	// Check 1-hour cache
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < 1*time.Hour {
			if data, err := os.ReadFile(cacheFile); err == nil {
				if tracks, err := ParseLikedMusicJSON(data); err == nil && len(tracks) > 0 {
					return tracks, nil
				}
			}
		}
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{"--flat-playlist", "-J", "https://www.youtube.com/playlist?list=LM"}
	if a.SignedIn() {
		if cookieCopy, err := a.PlaybackCookieFile(); err == nil {
			args = append([]string{"--cookies", cookieCopy}, args...)
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		cleanStderr := SanitizeErrorOutput(stderr.String())
		if isLoginSignal(cleanStderr) {
			return nil, fmt.Errorf("%w: %s", ErrLoginRequired, cleanStderr)
		}
		return nil, fmt.Errorf("fetching liked music: %w (%s)", err, cleanStderr)
	}

	outBytes := stdout.Bytes()
	tracks, err := ParseLikedMusicJSON(outBytes)
	if err != nil {
		return nil, err
	}

	// Atomically write cache
	_ = os.MkdirAll(cacheDir, 0o755)
	tmpFile := cacheFile + ".tmp"
	if err := os.WriteFile(tmpFile, outBytes, 0o644); err == nil {
		_ = os.Rename(tmpFile, cacheFile)
	}

	return tracks, nil
}

// SearchMusic executes a search on music.youtube.com via yt-dlp.
// Note: yt-dlp displays a warning that YouTube Music is not directly supported and redirects.
func (a *Account) SearchMusic(ctx context.Context, query string) (*Album, []Video, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, nil, errors.New("empty music query")
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	searchURL := fmt.Sprintf("https://music.youtube.com/search?q=%s", url.QueryEscape(trimmed))
	args := []string{"--flat-playlist", "-J", searchURL}
	if a.SignedIn() {
		if cookieCopy, err := a.PlaybackCookieFile(); err == nil {
			args = append([]string{"--cookies", cookieCopy}, args...)
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		cleanStderr := SanitizeErrorOutput(stderr.String())
		return nil, nil, fmt.Errorf("searching music: %w (%s)", err, cleanStderr)
	}

	return ParseMusicAlbumJSON(stdout.Bytes())
}

// Album fetches an album or playlist detail via yt-dlp.
func (a *Account) Album(ctx context.Context, albumTarget string) (*Album, []Video, error) {
	trimmed := strings.TrimSpace(albumTarget)
	if trimmed == "" {
		return nil, nil, errors.New("empty album target")
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{"--flat-playlist", "-J", trimmed}
	if a.SignedIn() {
		if cookieCopy, err := a.PlaybackCookieFile(); err == nil {
			args = append([]string{"--cookies", cookieCopy}, args...)
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		cleanStderr := SanitizeErrorOutput(stderr.String())
		return nil, nil, fmt.Errorf("fetching album: %w (%s)", err, cleanStderr)
	}

	return ParseMusicAlbumJSON(stdout.Bytes())
}
