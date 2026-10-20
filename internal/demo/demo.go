package demo

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"io/fs"
	"sync"
)

// DemoFiles lists the 12 embedded demo image filenames.
var DemoFiles = []string{
	"city.jpg",
	"workshop.jpg",
	"camera.jpg",
	"performance.jpg",
	"forest.jpg",
	"architecture.jpg",
	"vinyl.jpg",
	"microphone.jpg",
	"ocean.jpg",
	"night.jpg",
	"portrait-city.jpg",
	"portrait-forest.jpg",
}

//go:embed *.jpg ASSETS.md
var FS embed.FS

// AssetFS is the filesystem used to load demo assets. It defaults to the embedded FS,
// but can be replaced in tests to simulate missing or corrupted assets.
var AssetFS fs.FS = FS

// ValidateAssets checks that all required embedded demo files are present and valid.
// It ensures each file exists, is non-empty, and can be decoded as a valid image.
// In demo mode, if any embedded asset is missing or broken, the application must exit
// with an error and never fall back to real account or network calls.
func ValidateAssets() error {
	for _, f := range DemoFiles {
		data, err := fs.ReadFile(AssetFS, f)
		if err != nil {
			return fmt.Errorf("embedded demo asset %s is missing: %w", f, err)
		}
		if len(data) == 0 {
			return fmt.Errorf("embedded demo asset %s is empty", f)
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return fmt.Errorf("embedded demo asset %s is invalid image: %w", f, err)
		}
	}
	return nil
}

var (
	cacheMu sync.RWMutex
	cache   = make(map[string]image.Image)
)

// GetImage lazily decodes and caches an embedded demo image.
func GetImage(filename string) (image.Image, error) {
	cacheMu.RLock()
	if img, ok := cache[filename]; ok {
		cacheMu.RUnlock()
		return img, nil
	}
	cacheMu.RUnlock()

	data, err := fs.ReadFile(AssetFS, filename)
	if err != nil {
		return nil, fmt.Errorf("reading embedded demo file %s: %w", filename, err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding demo image %s: %w", filename, err)
	}

	// Share the decoded pixels with Gio instead of retaining both YCbCr and RGBA.
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	img = rgba
	cacheMu.Lock()
	if existing, ok := cache[filename]; ok {
		img = existing
	}
	cache[filename] = img
	cacheMu.Unlock()

	return img, nil
}

// ResetCache clears the demo image cache (useful in tests).
func ResetCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = make(map[string]image.Image)
}

// CachedImage returns the cached decoded image if already decoded, without blocking.
func CachedImage(filename string) (image.Image, bool) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	img, ok := cache[filename]
	return img, ok
}
