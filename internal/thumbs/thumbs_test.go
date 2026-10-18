package thumbs

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func generateTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func TestLRUCacheEviction(t *testing.T) {
	cache := newLRUCache(3)

	img1 := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img2 := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img3 := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img4 := image.NewRGBA(image.Rect(0, 0, 1, 1))

	cache.put("1", img1)
	cache.put("2", img2)
	cache.put("3", img3)

	if cache.len() != 3 {
		t.Fatalf("expected len 3, got %d", cache.len())
	}

	// Access "1" to make "2" the oldest.
	if _, ok := cache.get("1"); !ok {
		t.Errorf("expected 1 in cache")
	}

	// Adding "4" should evict "2".
	cache.put("4", img4)

	if _, ok := cache.get("2"); ok {
		t.Errorf("expected 2 to be evicted from cache")
	}
	if _, ok := cache.get("1"); !ok {
		t.Errorf("expected 1 still in cache")
	}
	if _, ok := cache.get("3"); !ok {
		t.Errorf("expected 3 still in cache")
	}
	if _, ok := cache.get("4"); !ok {
		t.Errorf("expected 4 in cache")
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestManagerFetchAndDiskCache(t *testing.T) {
	cacheDir := t.TempDir()
	jpegData := generateTestJPEG(320, 180)

	var networkCalls int32
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&networkCalls, 1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioNopCloser(bytes.NewReader(jpegData)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	mgr := NewWithOptions(client, cacheDir, 100)

	// Fetch thumbnail.
	img, err := mgr.Fetch(context.Background(), "testvid1")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	if img.Bounds().Dx() != 320 || img.Bounds().Dy() != 180 {
		t.Errorf("unexpected image bounds: %v", img.Bounds())
	}

	// Verify file was written to disk.
	diskPath := filepath.Join(cacheDir, "testvid1.jpg")
	if fi, err := os.Stat(diskPath); err != nil || fi.Size() == 0 {
		t.Fatalf("expected cached file on disk at %s", diskPath)
	}

	// Fast path: memory LRU hit.
	if cachedImg, ok := mgr.Get("testvid1"); !ok || cachedImg == nil {
		t.Errorf("expected testvid1 in memory LRU")
	}

	// Now create a new manager with same disk cache and empty LRU.
	// Network should NOT be called.
	mgr2 := NewWithOptions(client, cacheDir, 100)
	img2, err := mgr2.Fetch(context.Background(), "testvid1")
	if err != nil {
		t.Fatalf("Fetch from disk cache failed: %v", err)
	}
	if img2 == nil {
		t.Fatal("expected non-nil image from disk cache")
	}

	if atomic.LoadInt32(&networkCalls) != 1 {
		t.Errorf("expected exactly 1 network call, got %d", networkCalls)
	}
}

func TestManagerSingleFlight(t *testing.T) {
	cacheDir := t.TempDir()
	jpegData := generateTestJPEG(320, 180)

	var networkCalls int32
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&networkCalls, 1)
			time.Sleep(50 * time.Millisecond) // Simulate network delay
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioNopCloser(bytes.NewReader(jpegData)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	mgr := NewWithOptions(client, cacheDir, 100)

	var wg sync.WaitGroup
	results := make([]image.Image, 5)
	errors := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			img, err := mgr.Fetch(context.Background(), "concurrent_vid")
			results[idx] = img
			errors[idx] = err
		}()
	}

	wg.Wait()

	for i := 0; i < 5; i++ {
		if errors[i] != nil {
			t.Errorf("request %d failed: %v", i, errors[i])
		}
		if results[i] == nil {
			t.Errorf("request %d returned nil image", i)
		}
	}

	if calls := atomic.LoadInt32(&networkCalls); calls != 1 {
		t.Errorf("expected singleflight to make 1 network call, got %d", calls)
	}
}

func ioNopCloser(r *bytes.Reader) *readCloser {
	return &readCloser{r}
}

type readCloser struct {
	*bytes.Reader
}

func (rc *readCloser) Close() error {
	return nil
}
