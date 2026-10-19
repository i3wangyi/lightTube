package yt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseAccountPlaylistsJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "playlists.json"))
	if err != nil {
		t.Fatalf("reading playlists fixture: %v", err)
	}

	playlists, err := ParseAccountPlaylistsJSON(data)
	if err != nil {
		t.Fatalf("parsing playlists: %v", err)
	}

	if len(playlists) != 3 {
		t.Fatalf("expected 3 playlists, got %d", len(playlists))
	}

	if playlists[0].ID != "LL" || playlists[0].Title != "Liked videos" {
		t.Errorf("unexpected playlist[0]: %+v", playlists[0])
	}
	if playlists[0].VideoCount != 0 {
		t.Errorf("expected 0 video count for Liked videos, got %d", playlists[0].VideoCount)
	}
	if playlists[0].Channel != "" {
		t.Errorf("expected the \"View full playlist\" label to be dropped, got channel %q", playlists[0].Channel)
	}
	if playlists[0].ThumbnailURL == "" {
		t.Errorf("expected thumbnail URL for Liked videos")
	}

	if playlists[1].ID != "PLtestGpuProgramming00000000000001" || playlists[1].Title != "GPU Programming" {
		t.Errorf("unexpected playlist[1]: %+v", playlists[1])
	}
	if playlists[1].VideoCount != 15 {
		t.Errorf("expected 15 video count, got %d", playlists[1].VideoCount)
	}
	if playlists[1].Channel != "Tech Channel" {
		t.Errorf("expected channel Tech Channel, got %q", playlists[1].Channel)
	}

	if playlists[2].ID != "PLtestTritonKernels000000000000002" || playlists[2].Title != "Triton GPU Kernels 101" {
		t.Errorf("unexpected playlist[2]: %+v", playlists[2])
	}
}

func TestParsePlaylistDetailJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "playlist_detail.json"))
	if err != nil {
		t.Fatalf("reading playlist detail fixture: %v", err)
	}

	title, videos, err := ParsePlaylistDetailJSON(data)
	if err != nil {
		t.Fatalf("parsing playlist detail: %v", err)
	}

	if title != "Triton GPU Kernels 101" {
		t.Errorf("expected title 'Triton GPU Kernels 101', got %q", title)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	if videos[0].ID != "vidLesson01" || videos[0].Duration != 940 || videos[0].ViewCount != 7900 {
		t.Errorf("unexpected video[0]: %+v", videos[0])
	}
	if videos[1].ID != "vidLesson02" || videos[1].Duration != 1240 {
		t.Errorf("unexpected video[1]: %+v", videos[1])
	}
}

func TestCachedPlaylists(t *testing.T) {
	dir := t.TempDir()
	acc := NewAccountWithOptions(AccountOptions{
		CacheDir: dir,
	})

	cached, err := acc.CachedPlaylists()
	if err != nil {
		t.Fatalf("unexpected error on missing cache: %v", err)
	}
	if len(cached) != 0 {
		t.Fatalf("expected empty cached list, got %d", len(cached))
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "playlists.json"))
	if err != nil {
		t.Fatal(err)
	}
	pls, err := ParseAccountPlaylistsJSON(data)
	if err != nil {
		t.Fatal(err)
	}

	plsJSON, err := json.Marshal(pls)
	if err != nil {
		t.Fatal(err)
	}
	cacheFile := filepath.Join(dir, "playlists.json")
	if err := atomicWriteFile(cacheFile, plsJSON); err != nil {
		t.Fatal(err)
	}

	cached, err = acc.CachedPlaylists()
	if err != nil {
		t.Fatalf("failed reading cached playlists: %v", err)
	}
	if len(cached) != len(pls) {
		t.Fatalf("expected %d playlists, got %d", len(pls), len(cached))
	}
}

func TestPlaylistsRequiresSignIn(t *testing.T) {
	dir := t.TempDir()
	acc := NewAccountWithOptions(AccountOptions{
		CookieFile: filepath.Join(dir, "nonexistent-cookies.txt"),
		CacheDir:   dir,
	})

	_, err := acc.Playlists(context.Background())
	if err != ErrLoginRequired {
		t.Fatalf("expected ErrLoginRequired, got %v", err)
	}
}
