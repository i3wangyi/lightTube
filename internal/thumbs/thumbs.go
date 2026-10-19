package thumbs

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/image/draw"

	"lighttube/internal/platform"
)

const (
	// Must stay above the number of thumbnails the UI requests at once (visible
	// rows plus prefetch, all lists), or eviction and reload loop every frame.
	DefaultLRUCapacity = 100
	thumbnailURLFormat = "https://i.ytimg.com/vi/%s/mqdefault.jpg"
	maxThumbWidth      = 640
	maxThumbHeight     = 360
)

// DefaultCacheDir returns the standard thumbnail disk cache directory under platform CacheDir.
func DefaultCacheDir() string {
	return filepath.Join(platform.CacheDir(), "thumbs")
}

type lruEntry struct {
	key string
	img image.Image
}

type lruCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
	onEvict  func(string)
}

func newLRUCache(capacity int) *lruCache {
	if capacity <= 0 {
		capacity = DefaultLRUCapacity
	}
	return &lruCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (c *lruCache) get(key string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*lruEntry).img, true
	}
	return nil, false
}

func (c *lruCache) put(key string, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*lruEntry).img = img
		return
	}

	elem := c.order.PushFront(&lruEntry{key: key, img: img})
	c.items[key] = elem

	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			ent := oldest.Value.(*lruEntry)
			delete(c.items, ent.key)
			if c.onEvict != nil {
				c.onEvict(ent.key)
			}
		}
	}
}

func (c *lruCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Manager fetches, caches on disk, and decodes thumbnails into an in-memory LRU.
// It is UI-agnostic and returns standard image.Image.
type Manager struct {
	httpClient *http.Client
	cacheDir   string
	lru        *lruCache

	flightMu sync.Mutex
	epoch    uint64
	inFlight map[string][]func(image.Image, error)
}

// New creates a thumbnail Manager with system defaults.
func New() *Manager {
	return NewWithOptions(http.DefaultClient, DefaultCacheDir(), DefaultLRUCapacity)
}

// NewWithOptions creates a thumbnail Manager with customized dependencies.
func NewWithOptions(client *http.Client, cacheDir string, lruCapacity int) *Manager {
	if client == nil {
		client = http.DefaultClient
	}
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	return &Manager{
		httpClient: client,
		cacheDir:   cacheDir,
		lru:        newLRUCache(lruCapacity),
		inFlight:   make(map[string][]func(image.Image, error)),
	}
}

// SetOnEvict sets a callback invoked whenever an entry is evicted from the memory LRU.
func (m *Manager) SetOnEvict(fn func(key string)) {
	if m == nil || m.lru == nil {
		return
	}
	m.lru.mu.Lock()
	defer m.lru.mu.Unlock()
	m.lru.onEvict = fn
}

// Get synchronously returns the image from memory LRU if present.
func (m *Manager) Get(id string) (image.Image, bool) {
	return m.lru.get(id)
}

// Clear flushes all in-memory decoded images from the LRU cache.
func (m *Manager) Clear() {
	if m == nil || m.lru == nil {
		return
	}
	m.flightMu.Lock()
	m.epoch++
	m.inFlight = make(map[string][]func(image.Image, error))
	m.flightMu.Unlock()

	m.lru.mu.Lock()
	defer m.lru.mu.Unlock()
	if m.lru.onEvict != nil {
		for k := range m.lru.items {
			m.lru.onEvict(k)
		}
	}
	m.lru.items = make(map[string]*list.Element)
	m.lru.order.Init()
}

// Load requests a thumbnail asynchronously. Network fetching and image decoding
// execute off the caller goroutine. The callback is invoked when the image is ready.
func (m *Manager) Load(id string, callback func(image.Image, error)) {
	if id == "" {
		if callback != nil {
			callback(nil, errors.New("empty video ID"))
		}
		return
	}

	// Fast path: memory LRU hit.
	if img, ok := m.lru.get(id); ok {
		if callback != nil {
			callback(img, nil)
		}
		return
	}

	m.flightMu.Lock()
	epoch := m.epoch
	waiters, inProgress := m.inFlight[id]
	if callback != nil {
		m.inFlight[id] = append(waiters, callback)
	}
	if inProgress {
		m.flightMu.Unlock()
		return
	}
	m.inFlight[id] = []func(image.Image, error){}
	if callback != nil {
		m.inFlight[id] = append(m.inFlight[id], callback)
	}
	m.flightMu.Unlock()

	// Perform disk read/network fetch/decoding off the caller goroutine.
	go m.processThumbnail(epoch, id)
}

// Fetch blocks until the thumbnail is retrieved and decoded, executing work off the caller goroutine.
func (m *Manager) Fetch(ctx context.Context, id string) (image.Image, error) {
	type result struct {
		img image.Image
		err error
	}
	ch := make(chan result, 1)

	m.Load(id, func(img image.Image, err error) {
		ch <- result{img: img, err: err}
	})

	select {
	case res := <-ch:
		return res.img, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Manager) processThumbnail(epoch uint64, id string) {
	img, err := m.loadOrFetchAndDecode(id)

	m.flightMu.Lock()
	if epoch != m.epoch {
		// Previous page was dropped; discard thumbnail.
		delete(m.inFlight, id)
		m.flightMu.Unlock()
		return
	}
	waiters := m.inFlight[id]
	delete(m.inFlight, id)
	m.flightMu.Unlock()

	if err == nil && img != nil {
		m.lru.put(id, img)
	}

	for _, cb := range waiters {
		if cb != nil {
			cb(img, err)
		}
	}
}

func (m *Manager) loadOrFetchAndDecode(id string) (image.Image, error) {
	cacheFile := filepath.Join(m.cacheDir, id+".jpg")

	// 1. Try disk cache.
	if data, err := os.ReadFile(cacheFile); err == nil {
		img, _, decodeErr := image.Decode(bytes.NewReader(data))
		if decodeErr == nil {
			return decodeAndScale(img), nil
		}
	}

	// 2. Fetch from network.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	thumbURL := fmt.Sprintf(thumbnailURLFormat, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching thumbnail %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumbnail HTTP status %d for %s", resp.StatusCode, id)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading thumbnail body %s: %w", id, err)
	}

	// 3. Save to disk cache atomically.
	_ = atomicWriteFile(cacheFile, body)

	// 4. Decode off caller goroutine.
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decoding thumbnail %s: %w", id, err)
	}

	return decodeAndScale(img), nil
}

func decodeAndScale(src image.Image) image.Image {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return src
	}

	// Scale down if larger than maximum displayed size (640x360).
	if w > maxThumbWidth || h > maxThumbHeight {
		dstW := maxThumbWidth
		dstH := h * maxThumbWidth / w
		if dstH > maxThumbHeight {
			dstH = maxThumbHeight
			dstW = w * maxThumbHeight / h
		}
		if dstW < 1 {
			dstW = 1
		}
		if dstH < 1 {
			dstH = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
		draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
		return dst
	}

	// For standard thumbnails within 640x360 (e.g. 320x180), convert to *image.RGBA
	// once so Gio's paint.NewImageOp shares the identical buffer without allocating a copy.
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
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
