package thumbs

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"lighttube/internal/platform"
)

const (
	// DefaultLRUByteBudget is 48 MB as specified in DESIGN.md Phase 8.
	DefaultLRUByteBudget = 48 << 20
	// DefaultAvatarByteBudget is 8 MB for small avatar thumbnails.
	DefaultAvatarByteBudget = 8 << 20

	maxThumbWidth   = 640
	maxThumbHeight  = 360
	maxAvatarWidth  = 128
	maxAvatarHeight = 128

	// MaxThumbnailBytes is the maximum RGBA memory size for a 640x360 thumbnail.
	MaxThumbnailBytes = maxThumbWidth * maxThumbHeight * 4
	// MaxThumbnailsInHalfBudget is the maximum number of full-sized thumbnails that fit in half the LRU budget.
	MaxThumbnailsInHalfBudget = (DefaultLRUByteBudget / 2) / MaxThumbnailBytes
)

// DefaultCacheDir returns the standard thumbnail disk cache directory under platform CacheDir.
func DefaultCacheDir() string {
	return filepath.Join(platform.CacheDir(), "thumbs")
}

type lruEntry struct {
	key       string
	img       image.Image
	byteCount int
}

type lruCache struct {
	mu        sync.Mutex
	maxBytes  int
	usedBytes int
	items     map[string]*list.Element
	order     *list.List
	onEvict   func(string)
}

func newLRUCache(byteCapacity int) *lruCache {
	if byteCapacity <= 0 {
		byteCapacity = DefaultLRUByteBudget
	}
	return &lruCache{
		maxBytes: byteCapacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func imageBytes(img image.Image) int {
	if img == nil {
		return 0
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return len(rgba.Pix)
	}
	b := img.Bounds()
	return b.Dx() * b.Dy() * 4
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

	b := imageBytes(img)
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		old := elem.Value.(*lruEntry)
		c.usedBytes += b - old.byteCount
		old.img = img
		old.byteCount = b
	} else {
		elem := c.order.PushFront(&lruEntry{key: key, img: img, byteCount: b})
		c.items[key] = elem
		c.usedBytes += b
	}

	for c.usedBytes > c.maxBytes && c.order.Len() > 0 {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.order.Remove(oldest)
		ent := oldest.Value.(*lruEntry)
		delete(c.items, ent.key)
		c.usedBytes -= ent.byteCount
		if c.onEvict != nil {
			c.onEvict(ent.key)
		}
	}
}

func (c *lruCache) used() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usedBytes
}

func (c *lruCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Manager fetches, caches on disk, and decodes thumbnails and channel avatars into an in-memory LRU.
type Manager struct {
	httpClient *http.Client
	cacheDir   string
	lru        *lruCache
	avatarLRU  *lruCache

	flightMu sync.Mutex
	epoch    uint64
	inFlight map[string][]func(image.Image, error)

	avatarFlightMu sync.Mutex
	avatarInFlight map[string][]func(image.Image, error)

	loadCalls atomic.Int64
}

// New creates a thumbnail Manager with system defaults.
func New() *Manager {
	return NewWithOptions(http.DefaultClient, DefaultCacheDir(), DefaultLRUByteBudget)
}

// NewWithOptions creates a thumbnail Manager with customized dependencies.
func NewWithOptions(client *http.Client, cacheDir string, lruBytes int) *Manager {
	if client == nil {
		client = http.DefaultClient
	}
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	if lruBytes <= 0 {
		lruBytes = DefaultLRUByteBudget
	}
	return &Manager{
		httpClient:     client,
		cacheDir:       cacheDir,
		lru:            newLRUCache(lruBytes),
		avatarLRU:      newLRUCache(DefaultAvatarByteBudget),
		inFlight:       make(map[string][]func(image.Image, error)),
		avatarInFlight: make(map[string][]func(image.Image, error)),
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

// Get synchronously returns the thumbnail from memory LRU if present.
func (m *Manager) Get(id string) (image.Image, bool) {
	return m.lru.get(id)
}

// UsedBytes returns the current memory usage of the thumbnail LRU in bytes.
func (m *Manager) UsedBytes() int {
	if m == nil || m.lru == nil {
		return 0
	}
	return m.lru.used()
}

// MaxBytes returns the memory budget of the thumbnail LRU in bytes.
func (m *Manager) MaxBytes() int {
	if m == nil || m.lru == nil {
		return 0
	}
	return m.lru.maxBytes
}

// Len returns the number of thumbnails currently held in memory.
func (m *Manager) Len() int {
	if m == nil || m.lru == nil {
		return 0
	}
	return m.lru.len()
}

// Clear flushes all in-memory decoded images from the LRU caches.
func (m *Manager) Clear() {
	if m == nil || m.lru == nil {
		return
	}
	m.flightMu.Lock()
	m.epoch++
	m.inFlight = make(map[string][]func(image.Image, error))
	m.flightMu.Unlock()

	m.lru.mu.Lock()
	if m.lru.onEvict != nil {
		for k := range m.lru.items {
			m.lru.onEvict(k)
		}
	}
	m.lru.items = make(map[string]*list.Element)
	m.lru.order.Init()
	m.lru.usedBytes = 0
	m.lru.mu.Unlock()

	m.avatarFlightMu.Lock()
	m.avatarInFlight = make(map[string][]func(image.Image, error))
	m.avatarFlightMu.Unlock()

	m.avatarLRU.mu.Lock()
	m.avatarLRU.items = make(map[string]*list.Element)
	m.avatarLRU.order.Init()
	m.avatarLRU.usedBytes = 0
	m.avatarLRU.mu.Unlock()
}

// LoadCount returns the total number of Load calls received by this Manager.
func (m *Manager) LoadCount() int64 {
	if m == nil {
		return 0
	}
	return m.loadCalls.Load()
}

// Load requests a thumbnail asynchronously. Network fetching and image decoding
// execute off the caller goroutine. The callback is invoked when the image is ready.
func (m *Manager) Load(id string, callback func(image.Image, error)) {
	if m != nil {
		m.loadCalls.Add(1)
	}
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
	// Disk cache file is <id>-hq.jpg. Older <id>.jpg files are low-res and ignored.
	cacheFile := filepath.Join(m.cacheDir, id+"-hq.jpg")

	// 1. Try disk cache.
	if data, err := os.ReadFile(cacheFile); err == nil {
		img, _, decodeErr := image.Decode(bytes.NewReader(data))
		if decodeErr == nil {
			return decodeAndScale(img), nil
		}
	}

	// 2. Fetch order: hq720.jpg -> hqdefault.jpg cropped to 16:9 -> mqdefault.jpg.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	candidates := []string{
		fmt.Sprintf("https://i.ytimg.com/vi/%s/hq720.jpg", id),
		fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", id),
		fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", id),
	}

	for idx, u := range candidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}

		resp, err := m.httpClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		img, _, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			continue
		}

		if idx == 1 {
			// hqdefault.jpg is 4:3 with top/bottom black bars; crop to 16:9.
			cropped := cropHQDefault(img)
			var buf bytes.Buffer
			if encErr := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 90}); encErr == nil {
				_ = atomicWriteFile(cacheFile, buf.Bytes())
			}
			return decodeAndScale(cropped), nil
		}

		// Save the successful raw JPEG to <id>-hq.jpg atomically.
		_ = atomicWriteFile(cacheFile, body)
		return decodeAndScale(img), nil
	}

	return nil, fmt.Errorf("unable to fetch thumbnail for %s", id)
}

func cropHQDefault(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	targetH := w * 9 / 16
	if targetH >= h {
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
		return dst
	}
	barH := (h - targetH) / 2
	dst := image.NewRGBA(image.Rect(0, 0, w, targetH))
	cropMin := image.Pt(bounds.Min.X, bounds.Min.Y+barH)
	draw.Draw(dst, dst.Bounds(), src, cropMin, draw.Src)
	return dst
}

func decodeAndScale(src image.Image) *image.RGBA {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil
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

	// For standard thumbnails within 640x360, convert to *image.RGBA
	// once so Gio's paint.NewImageOp shares the identical buffer without allocating a copy.
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
}

// GetAvatar returns the channel avatar from the avatar LRU if available.
func (m *Manager) GetAvatar(rawURL string) (image.Image, bool) {
	if m == nil || m.avatarLRU == nil || rawURL == "" {
		return nil, false
	}
	return m.avatarLRU.get(rawURL)
}

// RequestAvatar requests a channel avatar asynchronously without a callback.
func (m *Manager) RequestAvatar(rawURL string) {
	m.LoadAvatar(rawURL, nil)
}

// LoadAvatar requests a channel avatar asynchronously (URL key, 128x128 max, disk cached).
func (m *Manager) LoadAvatar(rawURL string, callback func(image.Image, error)) {
	if rawURL == "" {
		if callback != nil {
			callback(nil, errors.New("empty avatar URL"))
		}
		return
	}

	if img, ok := m.avatarLRU.get(rawURL); ok {
		if callback != nil {
			callback(img, nil)
		}
		return
	}

	m.avatarFlightMu.Lock()
	waiters, inProgress := m.avatarInFlight[rawURL]
	if callback != nil {
		m.avatarInFlight[rawURL] = append(waiters, callback)
	}
	if inProgress {
		m.avatarFlightMu.Unlock()
		return
	}
	m.avatarInFlight[rawURL] = []func(image.Image, error){}
	if callback != nil {
		m.avatarInFlight[rawURL] = append(m.avatarInFlight[rawURL], callback)
	}
	m.avatarFlightMu.Unlock()

	go m.processAvatar(rawURL)
}

func (m *Manager) processAvatar(rawURL string) {
	img, err := m.loadOrFetchAvatar(rawURL)

	m.avatarFlightMu.Lock()
	waiters := m.avatarInFlight[rawURL]
	delete(m.avatarInFlight, rawURL)
	m.avatarFlightMu.Unlock()

	if err == nil && img != nil {
		m.avatarLRU.put(rawURL, img)
	}

	for _, cb := range waiters {
		if cb != nil {
			cb(img, err)
		}
	}
}

func (m *Manager) loadOrFetchAvatar(rawURL string) (image.Image, error) {
	h := sha256.Sum256([]byte(rawURL))
	hashStr := hex.EncodeToString(h[:16])
	avatarsDir := filepath.Join(m.cacheDir, "avatars")
	cacheFile := filepath.Join(avatarsDir, hashStr+".jpg")

	if data, err := os.ReadFile(cacheFile); err == nil {
		img, _, decodeErr := image.Decode(bytes.NewReader(data))
		if decodeErr == nil {
			return decodeAndScaleAvatar(img), nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching avatar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading avatar body: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decoding avatar: %w", err)
	}

	scaled := decodeAndScaleAvatar(img)
	var buf bytes.Buffer
	if encErr := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: 90}); encErr == nil {
		_ = atomicWriteFile(cacheFile, buf.Bytes())
	}

	return scaled, nil
}

func decodeAndScaleAvatar(src image.Image) *image.RGBA {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	if w > maxAvatarWidth || h > maxAvatarHeight {
		dstW := maxAvatarWidth
		dstH := h * maxAvatarWidth / w
		if dstH > maxAvatarHeight {
			dstH = maxAvatarHeight
			dstW = w * maxAvatarHeight / h
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
