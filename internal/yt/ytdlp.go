package yt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// YTDLP abstracts the external yt-dlp binary calls to enable fixture-based testing.
type YTDLP interface {
	Search(ctx context.Context, query string) ([]byte, error)
	ResolveChannel(ctx context.Context, target string) (string, error)
}

// ExecYTDLP executes yt-dlp via os/exec.
type ExecYTDLP struct {
	BinaryPath string
}

// NewExecYTDLP creates an ExecYTDLP instance with binary detection.
func NewExecYTDLP() *ExecYTDLP {
	return &ExecYTDLP{BinaryPath: "yt-dlp"}
}

// Search executes yt-dlp flat playlist search with a 15-second timeout.
func (e *ExecYTDLP) Search(ctx context.Context, query string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	bin := e.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	searchArg := fmt.Sprintf("ytsearch20:%s", query)
	cmd := exec.CommandContext(ctx, bin, "--flat-playlist", "-J", "--no-warnings", searchArg)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp search error: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// ResolveChannel resolves a channel URL, @handle, or raw UC id to a channel_id.
func (e *ExecYTDLP) ResolveChannel(ctx context.Context, target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", errors.New("empty channel target")
	}

	// Raw 24-character YouTube UC channel ID does not need resolution.
	if strings.HasPrefix(trimmed, "UC") && len(trimmed) == 24 {
		return trimmed, nil
	}

	normalized := trimmed
	if strings.HasPrefix(normalized, "@") {
		normalized = "https://www.youtube.com/" + normalized
	} else if strings.HasPrefix(normalized, "youtube.com/") || strings.HasPrefix(normalized, "www.youtube.com/") {
		normalized = "https://" + normalized
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	bin := e.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	// Use %(channel_id,playlist_channel_id)s to capture channel ID whether treated as video entry or playlist tab.
	cmd := exec.CommandContext(ctx, bin, "--flat-playlist", "--playlist-items", "1", "--print", "%(channel_id,playlist_channel_id)s", normalized)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("yt-dlp resolve error: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	output := strings.TrimSpace(stdout.String())
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "NA" {
			return line, nil
		}
	}

	return "", fmt.Errorf("could not resolve channel ID from %q", target)
}

type ytdlpSearchOutput struct {
	Entries []ytdlpEntry `json:"entries"`
}

type ytdlpEntry struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Channel    string   `json:"channel"`
	ChannelID  string   `json:"channel_id"`
	Uploader   string   `json:"uploader"`
	UploaderID string   `json:"uploader_id"`
	Duration   *float64 `json:"duration"`
	ViewCount  *float64 `json:"view_count"`
	UploadDate string   `json:"upload_date"`
	URL        string   `json:"url"`
}

// ParseSearchJSON parses raw JSON from yt-dlp flat playlist search.
func ParseSearchJSON(data []byte) ([]Video, error) {
	var output ytdlpSearchOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("decoding yt-dlp search JSON: %w", err)
	}

	videos := make([]Video, 0, len(output.Entries))
	for _, entry := range output.Entries {
		if entry.ID == "" {
			continue
		}

		channel := entry.Channel
		if channel == "" {
			channel = entry.Uploader
		}

		channelID := entry.ChannelID
		if channelID == "" {
			channelID = entry.UploaderID
		}

		durationSec := 0
		if entry.Duration != nil {
			durationSec = int(*entry.Duration)
		}

		var viewCount int64
		if entry.ViewCount != nil {
			viewCount = int64(*entry.ViewCount)
		}

		videoURL := entry.URL
		if videoURL == "" {
			videoURL = "https://www.youtube.com/watch?v=" + entry.ID
		}

		videos = append(videos, Video{
			ID:         entry.ID,
			Title:      entry.Title,
			Channel:    channel,
			ChannelID:  channelID,
			Duration:   durationSec,
			ViewCount:  viewCount,
			UploadDate: entry.UploadDate,
			URL:        videoURL,
		})
	}

	return videos, nil
}
