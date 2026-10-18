package yt

import (
	"context"
	"net/http"
)

// Client coordinates search, channel resolution, and feed fetching.
type Client struct {
	ytdlp      YTDLP
	httpClient *http.Client
	cacheDir   string
}

// NewClient creates a Client using system defaults.
func NewClient() *Client {
	return NewClientWithOptions(NewExecYTDLP(), http.DefaultClient, DefaultFeedCacheDir())
}

// NewClientWithOptions creates a Client with custom dependencies for testing.
func NewClientWithOptions(ytdlp YTDLP, httpClient *http.Client, cacheDir string) *Client {
	if ytdlp == nil {
		ytdlp = NewExecYTDLP()
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if cacheDir == "" {
		cacheDir = DefaultFeedCacheDir()
	}
	return &Client{
		ytdlp:      ytdlp,
		httpClient: httpClient,
		cacheDir:   cacheDir,
	}
}

// Search queries YouTube via yt-dlp flat playlist JSON.
func (c *Client) Search(ctx context.Context, query string) ([]Video, error) {
	data, err := c.ytdlp.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	return ParseSearchJSON(data)
}

// ResolveChannel resolves a channel URL, @handle, or raw UC id to a channel_id.
func (c *Client) ResolveChannel(ctx context.Context, target string) (string, error) {
	return c.ytdlp.ResolveChannel(ctx, target)
}

// Feed fetches and parses a channel feed, utilizing disk cache when fresh.
func (c *Client) Feed(ctx context.Context, channelID string) ([]Video, error) {
	return FetchFeed(ctx, c.httpClient, c.cacheDir, channelID)
}

// MergedFeed fetches multiple channel feeds concurrently and returns merged newest-first entries.
func (c *Client) MergedFeed(ctx context.Context, channelIDs []string) ([]Video, error) {
	return FetchMergedFeed(ctx, c.httpClient, c.cacheDir, channelIDs)
}
