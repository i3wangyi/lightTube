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

// PodcastShow represents a followed or discovered podcast show.
type PodcastShow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Genre        string `json:"genre,omitempty"`
}

// ParsePodcastPlaylistSearch parses playlist results from YouTube's playlist search filter.
func ParsePodcastPlaylistSearch(data []byte) ([]Playlist, error) {
	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decoding podcast playlist search: %w", err)
	}
	var out []Playlist
	for _, entry := range payload.Entries {
		id, title := cleanString(entry["id"]), cleanString(entry["title"])
		link := cleanString(entry["url"])
		if link == "" {
			link = cleanString(entry["webpage_url"])
		}
		isPlaylist := strings.Contains(link, "list=") || strings.EqualFold(cleanString(entry["_type"]), "playlist")
		if !isPlaylist {
			continue
		}
		if !strings.Contains(link, "list=") && id != "" {
			link = "https://www.youtube.com/playlist?list=" + id
		}
		if id == "" || title == "" || !strings.Contains(link, "list=") {
			continue
		}
		out = append(out, Playlist{ID: id, Title: title, URL: link, ThumbnailURL: cleanString(entry["thumbnail"])})
	}
	return out, nil
}

// SearchPodcastPlaylists searches YouTube's playlist-only result filter.
func (a *Account) SearchPodcastPlaylists(ctx context.Context, query string) ([]Playlist, error) {
	searchURL := "https://www.youtube.com/results?search_query=" + url.QueryEscape(query+" podcast") + "&sp=EgIQAw%3D%3D"
	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}
	args := []string{"--flat-playlist", "-J", searchURL}
	if a.SignedIn() {
		if cookies, err := a.PlaybackCookieFile(); err == nil {
			args = append([]string{"--cookies", cookies}, args...)
		}
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("searching podcast playlists: %w (%s)", err, SanitizeErrorOutput(stderr.String()))
	}
	return ParsePodcastPlaylistSearch(stdout.Bytes())
}

// ParsePodcastsFeedJSON parses the yt-dlp flat playlist output for https://www.youtube.com/feed/podcasts.
func ParsePodcastsFeedJSON(data []byte) ([]PodcastShow, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []PodcastShow{}, nil
	}

	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("decoding podcasts feed JSON: %w", err)
	}

	if len(payload.Entries) == 0 {
		return []PodcastShow{}, nil
	}

	shows := make([]PodcastShow, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		id := strings.TrimSpace(cleanString(entry["id"]))
		title := strings.TrimSpace(cleanString(entry["title"]))
		if id == "" && title == "" {
			continue
		}

		channel := cleanString(entry["channel"])
		if channel == "" {
			channel = cleanString(entry["uploader"])
		}
		// YouTube puts its "View full playlist" link label in the owner/channel field.
		if strings.EqualFold(channel, "View full playlist") {
			channel = ""
		}

		rawURL := cleanString(entry["url"])
		if rawURL == "" && id != "" {
			rawURL = "https://www.youtube.com/playlist?list=" + id
		}

		// Find best thumbnail
		thumbURL := ""
		if thumbs, ok := entry["thumbnails"].([]any); ok && len(thumbs) > 0 {
			var maxH int
			for _, item := range thumbs {
				if m, ok := item.(map[string]any); ok {
					u := cleanString(m["url"])
					h := cleanInt(m["height"])
					if u != "" && (thumbURL == "" || h > maxH) {
						thumbURL = u
						maxH = h
					}
				}
			}
		}

		shows = append(shows, PodcastShow{
			ID:           id,
			Title:        title,
			URL:          rawURL,
			ThumbnailURL: thumbURL,
			Channel:      channel,
		})
	}

	return shows, nil
}

// PodcastsFeed fetches https://www.youtube.com/feed/podcasts via yt-dlp, caching on disk for 1 hour.
func (a *Account) PodcastsFeed(ctx context.Context) ([]PodcastShow, error) {
	cacheDir := filepath.Join(a.CacheDir, "podcasts")
	cacheFile := filepath.Join(cacheDir, "feed.json")

	// Check 1-hour cache
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < 1*time.Hour {
			if data, err := os.ReadFile(cacheFile); err == nil {
				if shows, err := ParsePodcastsFeedJSON(data); err == nil && len(shows) > 0 {
					return shows, nil
				}
			}
		}
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{"--flat-playlist", "-J", "https://www.youtube.com/feed/podcasts"}
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
		return nil, fmt.Errorf("fetching podcasts feed: %w (%s)", err, cleanStderr)
	}

	outBytes := stdout.Bytes()
	shows, err := ParsePodcastsFeedJSON(outBytes)
	if err != nil {
		return nil, err
	}

	// Atomically write cache
	_ = os.MkdirAll(cacheDir, 0o755)
	tmpFile := cacheFile + ".tmp"
	if err := os.WriteFile(tmpFile, outBytes, 0o644); err == nil {
		_ = os.Rename(tmpFile, cacheFile)
	}

	return shows, nil
}

// ShowEpisodes retrieves all ordered episode videos for a podcast show playlist.
func (a *Account) ShowEpisodes(ctx context.Context, playlistURL string) ([]Video, error) {
	if playlistURL == "" {
		return nil, errors.New("empty podcast playlist URL")
	}

	_, videos, err := a.PlaylistVideos(ctx, playlistURL)
	if err != nil {
		return nil, err
	}
	return videos, nil
}
