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

// ShortsSearchURL constructs the YouTube search URL scoped to Shorts with filter sp=EgIQCQ%3D%3D.
func ShortsSearchURL(query string) string {
	return fmt.Sprintf("https://www.youtube.com/results?search_query=%s&sp=EgIQCQ%%3D%%3D", url.QueryEscape(query))
}

// ParseShortsTabJSON parses the JSON output of yt-dlp --flat-playlist -J for a channel's /shorts tab.
// It assigns ProvenanceShortsTab to all parsed items.
func ParseShortsTabJSON(data []byte) ([]Video, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []Video{}, nil
	}

	var payload struct {
		ID      string           `json:"id"`
		Title   string           `json:"title"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("decoding shorts tab JSON: %w", err)
	}

	if len(payload.Entries) == 0 {
		return []Video{}, nil
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
		desc := cleanString(entry["description"])

		videoURL := cleanString(entry["url"])
		if videoURL == "" {
			videoURL = "https://www.youtube.com/shorts/" + id
		}

		videos = append(videos, Video{
			ID:          id,
			Title:       title,
			Channel:     channel,
			ChannelID:   channelID,
			Duration:    durationSec,
			ViewCount:   viewCount,
			UploadDate:  uploadDate,
			URL:         videoURL,
			Description: desc,
		})
	}

	return videos, nil
}

// ParseShortsSearchJSON parses yt-dlp flat-playlist output for a Shorts filter search query.
func ParseShortsSearchJSON(data []byte) ([]Video, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []Video{}, nil
	}

	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("decoding shorts search JSON: %w", err)
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
		desc := cleanString(entry["description"])

		videoURL := cleanString(entry["url"])
		if videoURL == "" {
			videoURL = "https://www.youtube.com/shorts/" + id
		}

		videos = append(videos, Video{
			ID:          id,
			Title:       title,
			Channel:     channel,
			ChannelID:   channelID,
			Duration:    durationSec,
			ViewCount:   viewCount,
			UploadDate:  uploadDate,
			URL:         videoURL,
			Description: desc,
		})
	}

	return videos, nil
}

// FetchChannelShorts retrieves a channel's /shorts tab via yt-dlp, caching on disk for 1 hour.
func (a *Account) FetchChannelShorts(ctx context.Context, channelTarget string) ([]Video, error) {
	trimmed := strings.TrimSpace(channelTarget)
	if trimmed == "" {
		return nil, errors.New("empty channel target")
	}

	// Determine cache key
	cacheKey := trimmed
	if u, err := ExtractVideoID(trimmed); err == nil && u != "" {
		cacheKey = u
	} else {
		cacheKey = strings.ReplaceAll(cacheKey, "/", "_")
		cacheKey = strings.ReplaceAll(cacheKey, ":", "_")
	}

	cacheDir := filepath.Join(a.CacheDir, "shorts")
	cacheFile := filepath.Join(cacheDir, cacheKey+".json")

	// Check 1h cache
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < 1*time.Hour {
			if data, err := os.ReadFile(cacheFile); err == nil {
				if vids, err := ParseShortsTabJSON(data); err == nil && len(vids) > 0 {
					return vids, nil
				}
			}
		}
	}

	targetURL := trimmed
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		if strings.HasPrefix(targetURL, "@") {
			targetURL = "https://www.youtube.com/" + targetURL
		} else if strings.HasPrefix(targetURL, "UC") {
			targetURL = "https://www.youtube.com/channel/" + targetURL
		} else {
			targetURL = "https://www.youtube.com/" + targetURL
		}
	}
	if !strings.HasSuffix(targetURL, "/shorts") {
		targetURL = strings.TrimSuffix(targetURL, "/") + "/shorts"
	}

	bin := a.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{"--flat-playlist", "-J", targetURL}
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
		return nil, fmt.Errorf("fetching channel shorts: %w (%s)", err, cleanStderr)
	}

	outBytes := stdout.Bytes()
	vids, err := ParseShortsTabJSON(outBytes)
	if err != nil {
		return nil, err
	}

	// Atomically write cache
	_ = os.MkdirAll(cacheDir, 0o755)
	tmpFile := cacheFile + ".tmp"
	if err := os.WriteFile(tmpFile, outBytes, 0o644); err == nil {
		_ = os.Rename(tmpFile, cacheFile)
	}

	return vids, nil
}

// MergedShorts fetches and merges shorts from up to maxChannels followed channels, newest first.
func (a *Account) MergedShorts(ctx context.Context, maxChannels int) ([]Video, error) {
	if maxChannels <= 0 {
		maxChannels = 15
	}

	channels, err := a.CachedChannels()
	if err != nil || len(channels) == 0 {
		channels, err = a.Channels(ctx)
		if err != nil {
			return nil, err
		}
	}

	if len(channels) > maxChannels {
		channels = channels[:maxChannels]
	}

	var allShorts []Video
	seenIDs := make(map[string]bool)

	for _, ch := range channels {
		target := ch.URL
		if target == "" {
			target = ch.ID
		}
		vids, err := a.FetchChannelShorts(ctx, target)
		if err != nil {
			continue
		}
		for _, v := range vids {
			if !seenIDs[v.ID] {
				seenIDs[v.ID] = true
				allShorts = append(allShorts, v)
			}
		}
	}

	return allShorts, nil
}
