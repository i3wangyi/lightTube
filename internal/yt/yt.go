package yt

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Video represents a YouTube video item.
type Video struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Channel     string    `json:"channel"`
	ChannelID   string    `json:"channel_id"`
	Duration    int       `json:"duration"` // in seconds
	ViewCount   int64     `json:"view_count,omitempty"`
	UploadDate  string    `json:"upload_date,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	URL         string    `json:"url,omitempty"`
	Description string    `json:"description,omitempty"`
}

// WatchURL returns the standard watch URL for the video.
func (v Video) WatchURL() string {
	if v.URL != "" {
		return v.URL
	}
	return "https://www.youtube.com/watch?v=" + v.ID
}

// Subscription represents a subscribed YouTube channel.
type Subscription struct {
	ChannelID string `json:"channel_id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
}

// Channel represents an account-subscribed YouTube channel.
type Channel struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	FollowerCount int64  `json:"follower_count,omitempty"`
	URL           string `json:"url,omitempty"`
}

// Storyboard represents preview storyboard metadata for a video.
type Storyboard struct {
	VideoID  string   `json:"video_id"`
	FormatID string   `json:"format_id"`
	Rows     int      `json:"rows"`
	Columns  int      `json:"columns"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Sheets   []string `json:"sheets"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)

// ExtractVideoID extracts an 11-character YouTube video ID from a URL, path, or raw ID.
// It accepts formats including:
// - raw 11-char ID
// - https://www.youtube.com/watch?v=<id>
// - https://youtu.be/<id>
// - https://www.youtube.com/shorts/<id>
// - https://www.youtube.com/live/<id>
// - https://www.youtube.com/embed/<id>
func ExtractVideoID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty video URL or ID")
	}

	// Direct 11-character video ID.
	if idPattern.MatchString(trimmed) {
		return trimmed, nil
	}

	rawURL := trimmed
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}

	host := strings.ToLower(parsed.Host)
	// Strip standard port if present.
	if colon := strings.IndexByte(host, ':'); colon != -1 {
		host = host[:colon]
	}

	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")

	// youtu.be/<id>
	if host == "youtu.be" {
		if len(parts) > 0 && idPattern.MatchString(parts[0]) {
			return parts[0], nil
		}
	}

	// youtube.com variants
	if strings.HasSuffix(host, "youtube.com") {
		// /watch?v=<id>
		if len(parts) > 0 && parts[0] == "watch" {
			v := parsed.Query().Get("v")
			if idPattern.MatchString(v) {
				return v, nil
			}
		}

		// /shorts/<id>, /live/<id>, /embed/<id>, /v/<id>
		if len(parts) >= 2 {
			switch parts[0] {
			case "shorts", "live", "embed", "v":
				if idPattern.MatchString(parts[1]) {
					return parts[1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not extract YouTube video ID from %q", raw)
}

// ParseTakeoutCSV parses Google Takeout subscriptions.csv (columns: Channel Id, Channel Url, Channel Title).
func ParseTakeoutCSV(r io.Reader) ([]Subscription, error) {
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	idCol := -1
	urlCol := -1
	titleCol := -1

	for i, col := range header {
		clean := strings.ToLower(strings.TrimSpace(col))
		switch clean {
		case "channel id":
			idCol = i
		case "channel url":
			urlCol = i
		case "channel title":
			titleCol = i
		}
	}

	if idCol == -1 || titleCol == -1 {
		return nil, errors.New("CSV missing required columns (Channel Id, Channel Title)")
	}

	var subs []Subscription
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV record: %w", err)
		}

		if idCol >= len(record) || titleCol >= len(record) {
			continue
		}

		channelID := strings.TrimSpace(record[idCol])
		title := strings.TrimSpace(record[titleCol])
		var channelURL string
		if urlCol != -1 && urlCol < len(record) {
			channelURL = strings.TrimSpace(record[urlCol])
		}
		if channelURL == "" && channelID != "" {
			channelURL = "https://www.youtube.com/channel/" + channelID
		}

		if channelID != "" {
			subs = append(subs, Subscription{
				ChannelID: channelID,
				Title:     title,
				URL:       channelURL,
			})
		}
	}

	return subs, nil
}

// FormatDuration formats an integer duration in seconds into MM:SS or H:MM:SS.
func FormatDuration(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
