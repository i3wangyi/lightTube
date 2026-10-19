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
	"strings"
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

func TestLRUCacheEvictionByBytes(t *testing.T) {
	// Each 1x1 image is 4 bytes. Budget 12 bytes holds 3 images.
	cache := newLRUCache(12)

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
	if cache.used() != 12 {
		t.Fatalf("expected used 12 bytes, got %d", cache.used())
	}

	// Access "1" to make "2" the oldest.
	if _, ok := cache.get("1"); !ok {
		t.Errorf("expected 1 in cache")
	}

	// Adding "4" (4 bytes) brings total to 16 > 12 bytes, evicting "2".
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
	if cache.used() != 12 {
		t.Errorf("expected used 12 bytes after eviction, got %d", cache.used())
	}
}

func TestLRUCacheOnEvict(t *testing.T) {
	// Budget 8 bytes holds 2 images of 4 bytes each.
	cache := newLRUCache(8)
	var evicted []string
	cache.onEvict = func(key string) {
		evicted = append(evicted, key)
	}

	cache.put("a", image.NewRGBA(image.Rect(0, 0, 1, 1)))
	cache.put("b", image.NewRGBA(image.Rect(0, 0, 1, 1)))
	cache.put("c", image.NewRGBA(image.Rect(0, 0, 1, 1)))

	if len(evicted) != 1 || evicted[0] != "a" {
		t.Errorf("expected [a] evicted, got %v", evicted)
	}
}

func TestDecodeAndScale(t *testing.T) {
	// A 1280x720 image should scale down to 640x360.
	large := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	scaled := decodeAndScale(large)
	if scaled.Bounds().Dx() != 640 || scaled.Bounds().Dy() != 360 {
		t.Errorf("expected 640x360, got %v", scaled.Bounds())
	}

	// A 320x180 image should retain its dimensions.
	standard := image.NewRGBA(image.Rect(0, 0, 320, 180))
	res := decodeAndScale(standard)
	if res.Bounds().Dx() != 320 || res.Bounds().Dy() != 180 {
		t.Errorf("expected 320x180, got %v", res.Bounds())
	}
}

func TestCropHQDefault(t *testing.T) {
	// 480x360 should be cropped to 480x270 (16:9).
	src := image.NewRGBA(image.Rect(0, 0, 480, 360))
	cropped := cropHQDefault(src)
	if cropped.Bounds().Dx() != 480 || cropped.Bounds().Dy() != 270 {
		t.Errorf("expected 480x270, got %dx%d", cropped.Bounds().Dx(), cropped.Bounds().Dy())
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestManagerFetchOrder_HQ720First(t *testing.T) {
	cacheDir := t.TempDir()
	hq720Data := generateTestJPEG(1280, 720)

	var requestedURLs []string
	var mu sync.Mutex
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			requestedURLs = append(requestedURLs, req.URL.String())
			mu.Unlock()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioNopCloser(bytes.NewReader(hq720Data)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	mgr := NewWithOptions(client, cacheDir, DefaultLRUByteBudget)
	img, err := mgr.Fetch(context.Background(), "test_hq720")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}

	// Must scale 1280x720 down to max 640x360.
	if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Errorf("expected 640x360 scaled image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Verify hq720 was requested first and disk cache is <id>-hq.jpg.
	if len(requestedURLs) != 1 || !strings.Contains(requestedURLs[0], "hq720.jpg") {
		t.Errorf("expected hq720.jpg requested, got %v", requestedURLs)
	}

	diskPath := filepath.Join(cacheDir, "test_hq720-hq.jpg")
	if fi, err := os.Stat(diskPath); err != nil || fi.Size() == 0 {
		t.Fatalf("expected cached file at %s", diskPath)
	}
}

func TestManagerFetchOrder_FallbackToCropHQDefault(t *testing.T) {
	cacheDir := t.TempDir()
	hqDefaultData := generateTestJPEG(480, 360)

	var requestedURLs []string
	var mu sync.Mutex
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			requestedURLs = append(requestedURLs, req.URL.String())
			mu.Unlock()

			if strings.Contains(req.URL.String(), "hq720.jpg") {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       ioNopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioNopCloser(bytes.NewReader(hqDefaultData)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	mgr := NewWithOptions(client, cacheDir, DefaultLRUByteBudget)
	img, err := mgr.Fetch(context.Background(), "test_fallback_crop")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}

	// 480x360 cropped to 16:9 is 480x270.
	if img.Bounds().Dx() != 480 || img.Bounds().Dy() != 270 {
		t.Errorf("expected 480x270 cropped image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	if len(requestedURLs) != 2 {
		t.Fatalf("expected 2 requests, got %v", requestedURLs)
	}
	if !strings.Contains(requestedURLs[0], "hq720.jpg") || !strings.Contains(requestedURLs[1], "hqdefault.jpg") {
		t.Errorf("unexpected request order: %v", requestedURLs)
	}
}

func TestPrefetchNeverExceedsHalfBudget(t *testing.T) {
	// A 4-column grid with prefetch margin:
	// Verify that for various visible row counts, clamped prefetch thumbnails never exceed half the budget.
	halfBudget := DefaultLRUByteBudget / 2

	for visibleRows := 1; visibleRows <= 20; visibleRows++ {
		columns := 4
		marginRows := 1
		totalRows := visibleRows + 2*marginRows
		requestedCount := totalRows * columns

		if requestedCount > MaxThumbnailsInHalfBudget {
			requestedCount = MaxThumbnailsInHalfBudget
		}

		maxEstimatedBytes := requestedCount * MaxThumbnailBytes
		if maxEstimatedBytes > halfBudget {
			t.Fatalf("visibleRows=%d: requestedCount=%d produced %d bytes, exceeds half budget %d",
				visibleRows, requestedCount, maxEstimatedBytes, halfBudget)
		}
	}
}

func TestManagerSingleFlight(t *testing.T) {
	cacheDir := t.TempDir()
	jpegData := generateTestJPEG(320, 180)

	var networkCalls int32
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&networkCalls, 1)
			time.Sleep(50 * time.Millisecond)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioNopCloser(bytes.NewReader(jpegData)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	mgr := NewWithOptions(client, cacheDir, DefaultLRUByteBudget)

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

func TestAvatarManager(t *testing.T) {
	cacheDir := t.TempDir()
	avatarData := generateTestJPEG(256, 256)

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioNopCloser(bytes.NewReader(avatarData)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	mgr := NewWithOptions(client, cacheDir, DefaultLRUByteBudget)

	var wg sync.WaitGroup
	wg.Add(1)
	var loadedImg image.Image
	var loadErr error

	mgr.LoadAvatar("https://example.com/avatar.jpg", func(img image.Image, err error) {
		loadedImg = img
		loadErr = err
		wg.Done()
	})

	wg.Wait()

	if loadErr != nil {
		t.Fatalf("LoadAvatar failed: %v", loadErr)
	}
	if loadedImg == nil {
		t.Fatal("expected non-nil avatar image")
	}
	// Max avatar size 128x128.
	if loadedImg.Bounds().Dx() > 128 || loadedImg.Bounds().Dy() > 128 {
		t.Errorf("expected <= 128x128 avatar, got %dx%d", loadedImg.Bounds().Dx(), loadedImg.Bounds().Dy())
	}

	// Verify hit from in-memory LRU.
	cached, ok := mgr.GetAvatar("https://example.com/avatar.jpg")
	if !ok || cached == nil {
		t.Errorf("expected avatar in cache")
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
