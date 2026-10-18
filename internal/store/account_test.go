package store_test

import (
	"path/filepath"
	"testing"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

func TestAccountFeedCache(t *testing.T) {
	tempDir := t.TempDir()

	// Initial read should return nil, nil when no cache exists.
	videos, err := store.ReadAccountFeedWithDir(tempDir, "home")
	if err != nil {
		t.Fatalf("expected nil error on missing cache, got: %v", err)
	}
	if len(videos) != 0 {
		t.Fatalf("expected empty videos on missing cache, got: %v", videos)
	}

	sampleVideos := []yt.Video{
		{
			ID:        "v1",
			Title:     "Sample Video 1",
			Channel:   "Channel 1",
			Duration:  100,
			ViewCount: 500,
		},
		{
			ID:        "v2",
			Title:     "Sample Video 2",
			Channel:   "Channel 2",
			Duration:  200,
			ViewCount: 1000,
		},
	}

	// Save to cache.
	if err := store.SaveAccountFeedWithDir(tempDir, "home", sampleVideos); err != nil {
		t.Fatalf("SaveAccountFeedWithDir failed: %v", err)
	}

	// Read back from cache.
	loaded, err := store.ReadAccountFeedWithDir(tempDir, "home")
	if err != nil {
		t.Fatalf("ReadAccountFeedWithDir failed: %v", err)
	}
	if len(loaded) != len(sampleVideos) {
		t.Fatalf("expected %d loaded videos, got %d", len(sampleVideos), len(loaded))
	}
	if loaded[0].ID != sampleVideos[0].ID || loaded[0].Title != sampleVideos[0].Title {
		t.Errorf("loaded[0] %v does not match sample[0] %v", loaded[0], sampleVideos[0])
	}
	if loaded[1].ID != sampleVideos[1].ID || loaded[1].Title != sampleVideos[1].Title {
		t.Errorf("loaded[1] %v does not match sample[1] %v", loaded[1], sampleVideos[1])
	}

	// Check invalid empty kind.
	if err := store.SaveAccountFeedWithDir(tempDir, "", sampleVideos); err == nil {
		t.Fatal("expected error on empty feed kind, got nil")
	}
	if _, err := store.ReadAccountFeedWithDir(tempDir, ""); err == nil {
		t.Fatal("expected error on empty feed kind, got nil")
	}
}

func TestDefaultAccountCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	dir := store.DefaultAccountCacheDir()
	expected := filepath.Join("/custom/cache", "lighttube", "account")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}
