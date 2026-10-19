package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"lighttube/internal/platform"
	"lighttube/internal/yt"
)

// DefaultAccountCacheDir returns the standard account feed cache directory under platform CacheDir.
func DefaultAccountCacheDir() string {
	return filepath.Join(platform.CacheDir(), "account")
}

// ReadAccountFeed reads a cached account feed from the default cache directory.
// If the cache file does not exist, it returns nil, nil.
func ReadAccountFeed(kind string) ([]yt.Video, error) {
	return ReadAccountFeedWithDir(DefaultAccountCacheDir(), kind)
}

// ReadAccountFeedWithDir reads a cached account feed from the specified directory.
func ReadAccountFeedWithDir(dir, kind string) ([]yt.Video, error) {
	if kind == "" {
		return nil, errors.New("empty feed kind")
	}
	filePath := filepath.Join(dir, kind+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading account feed cache %s: %w", kind, err)
	}

	var videos []yt.Video
	if err := json.Unmarshal(data, &videos); err != nil {
		return nil, fmt.Errorf("decoding account feed cache %s: %w", kind, err)
	}
	return videos, nil
}

// SaveAccountFeed writes an account feed cache atomically to the default cache directory.
func SaveAccountFeed(kind string, videos []yt.Video) error {
	return SaveAccountFeedWithDir(DefaultAccountCacheDir(), kind, videos)
}

// SaveAccountFeedWithDir writes an account feed cache atomically to the specified directory.
func SaveAccountFeedWithDir(dir, kind string, videos []yt.Video) error {
	if kind == "" {
		return errors.New("empty feed kind")
	}
	filePath := filepath.Join(dir, kind+".json")
	return atomicWriteJSON(filePath, videos)
}
