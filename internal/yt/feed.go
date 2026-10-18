package yt

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultFeedCacheDir returns the standard feed cache directory under XDG_CACHE_HOME.
func DefaultFeedCacheDir() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "lighttube", "feeds")
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	VideoID   string     `xml:"videoId"`
	ChannelID string     `xml:"channelId"`
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Author    atomAuthor `xml:"author"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Media     atomMedia  `xml:"group"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

type atomMedia struct {
	Community atomCommunity `xml:"community"`
}

type atomCommunity struct {
	Statistics atomStatistics `xml:"statistics"`
}

type atomStatistics struct {
	Views int64 `xml:"views,attr"`
}

// ParseFeedXML parses raw YouTube Atom XML bytes into a slice of Videos.
// It hides YouTube Shorts when the link contains "/shorts/".
func ParseFeedXML(data []byte) ([]Video, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing feed XML: %w", err)
	}

	videos := make([]Video, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		videoID := entry.VideoID
		if videoID == "" && strings.HasPrefix(entry.ID, "yt:video:") {
			videoID = strings.TrimPrefix(entry.ID, "yt:video:")
		}
		if videoID == "" {
			continue
		}

		var href string
		for _, link := range entry.Links {
			if link.Rel == "alternate" || href == "" {
				href = link.Href
			}
		}
		if href == "" {
			href = "https://www.youtube.com/watch?v=" + videoID
		}

		// Hide YouTube Shorts when the feed entry link contains /shorts/.
		if strings.Contains(href, "/shorts/") {
			continue
		}

		var pubTime time.Time
		if entry.Published != "" {
			if t, err := time.Parse(time.RFC3339, entry.Published); err == nil {
				pubTime = t
			}
		}

		channel := entry.Author.Name
		if channel == "" {
			channel = feed.Title
		}

		videos = append(videos, Video{
			ID:          videoID,
			Title:       entry.Title,
			Channel:     channel,
			ChannelID:   entry.ChannelID,
			ViewCount:   entry.Media.Community.Statistics.Views,
			PublishedAt: pubTime,
			URL:         href,
		})
	}

	return videos, nil
}

// FetchFeed fetches and parses a channel's Atom RSS feed, caching it on disk for 15 minutes.
func FetchFeed(ctx context.Context, client *http.Client, cacheDir, channelID string) ([]Video, error) {
	if channelID == "" {
		return nil, errors.New("empty channel ID")
	}

	if cacheDir == "" {
		cacheDir = DefaultFeedCacheDir()
	}

	cacheFile := filepath.Join(cacheDir, channelID+".xml")

	// Check if cached file is younger than 15 minutes.
	if fi, err := os.Stat(cacheFile); err == nil {
		if time.Since(fi.ModTime()) < 15*time.Minute {
			data, readErr := os.ReadFile(cacheFile)
			if readErr == nil {
				return ParseFeedXML(data)
			}
		}
	}

	// Fetch from network with 10s timeout.
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	httpClient := client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// Fallback to expired cache if available during network failure.
		if data, readErr := os.ReadFile(cacheFile); readErr == nil {
			return ParseFeedXML(data)
		}
		return nil, fmt.Errorf("fetching feed for %s: %w", channelID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fallback to cache on non-200 if present.
		if data, readErr := os.ReadFile(cacheFile); readErr == nil {
			return ParseFeedXML(data)
		}
		return nil, fmt.Errorf("unexpected status %d fetching feed for %s", resp.StatusCode, channelID)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading feed response body for %s: %w", channelID, err)
	}

	// Cache to disk atomically.
	_ = atomicWriteFile(cacheFile, data)

	return ParseFeedXML(data)
}

// FetchMergedFeed fetches feeds for all channels in parallel (max 8 at once, 10s per request)
// and returns merged entries sorted newest-first without duplicates.
func FetchMergedFeed(ctx context.Context, client *http.Client, cacheDir string, channelIDs []string) ([]Video, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allVideos []Video

	for _, chID := range channelIDs {
		trimmedID := strings.TrimSpace(chID)
		if trimmedID == "" {
			continue
		}

		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			videos, err := FetchFeed(ctx, client, cacheDir, id)
			if err == nil {
				mu.Lock()
				allVideos = append(allVideos, videos...)
				mu.Unlock()
			}
		}(trimmedID)
	}

	wg.Wait()

	// Sort newest-first.
	sort.Slice(allVideos, func(i, j int) bool {
		return allVideos[i].PublishedAt.After(allVideos[j].PublishedAt)
	})

	// Deduplicate by video ID.
	seen := make(map[string]bool, len(allVideos))
	unique := make([]Video, 0, len(allVideos))
	for _, v := range allVideos {
		if seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		unique = append(unique, v)
	}

	return unique, nil
}

func atomicWriteFile(dest string, data []byte) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(dest)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, dest)
}
