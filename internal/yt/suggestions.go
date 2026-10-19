package yt

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSuggestURL      = "https://suggestqueries.google.com/complete/search?client=firefox&ds=yt&q="
	DefaultSuggestDebounce = 150 * time.Millisecond
	MaxSuggestionsCache    = 200
	MaxSuggestionsCount    = 10
)

// Clock abstracts timer creation for debounce logic without sleeps in tests.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer abstracts a stoppable timer.
type Timer interface {
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}

// SuggestionsCache is a bounded LRU cache for suggestion lists keyed by query string.
type SuggestionsCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	evict    *list.List
}

type cacheEntry struct {
	key   string
	value []string
}

// NewSuggestionsCache creates an LRU cache with the specified capacity.
func NewSuggestionsCache(capacity int) *SuggestionsCache {
	if capacity <= 0 {
		capacity = MaxSuggestionsCache
	}
	return &SuggestionsCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		evict:    list.New(),
	}
}

// Get returns the cached suggestions for key if present.
func (c *SuggestionsCache) Get(key string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evict.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		res := make([]string, len(entry.value))
		copy(res, entry.value)
		return res, true
	}
	return nil, false
}

// Put stores suggestions for key, evicting the least recently used entry if at capacity.
func (c *SuggestionsCache) Put(key string, values []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evict.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = values
		return
	}

	if c.evict.Len() >= c.capacity {
		oldest := c.evict.Back()
		if oldest != nil {
			c.evict.Remove(oldest)
			delete(c.items, oldest.Value.(*cacheEntry).key)
		}
	}

	copied := make([]string, len(values))
	copy(copied, values)
	elem := c.evict.PushFront(&cacheEntry{key: key, value: copied})
	c.items[key] = elem
}

// Len returns the current number of cached entries.
func (c *SuggestionsCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evict.Len()
}

// SuggestionsClient fetches search typeahead suggestions.
type SuggestionsClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Cache      *SuggestionsCache
}

// NewSuggestionsClient creates a standard suggestions client.
func NewSuggestionsClient(client *http.Client) *SuggestionsClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &SuggestionsClient{
		BaseURL:    DefaultSuggestURL,
		HTTPClient: client,
		Cache:      NewSuggestionsCache(MaxSuggestionsCache),
	}
}

// IsURLQuery returns true if query is empty or represents a direct URL or video ID.
func IsURLQuery(query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return true
	}
	if strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://") || strings.HasPrefix(q, "youtu.be/") {
		return true
	}
	if id, err := ExtractVideoID(q); err == nil && id != "" {
		return true
	}
	return false
}

// Fetch queries the suggestions endpoint for query, consulting the LRU cache first.
func (c *SuggestionsClient) Fetch(ctx context.Context, query string) ([]string, error) {
	trimmed := strings.TrimSpace(query)
	if IsURLQuery(trimmed) {
		return nil, nil
	}

	if c.Cache != nil {
		if cached, ok := c.Cache.Get(trimmed); ok {
			return cached, nil
		}
	}

	reqURL := c.BaseURL + url.QueryEscape(trimmed)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating suggestions request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching suggestions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("suggestions status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading suggestions response: %w", err)
	}

	suggestions, err := ParseSuggestionsJSON(body)
	if err != nil {
		return nil, err
	}

	if len(suggestions) > MaxSuggestionsCount {
		suggestions = suggestions[:MaxSuggestionsCount]
	}

	if c.Cache != nil {
		c.Cache.Put(trimmed, suggestions)
	}

	return suggestions, nil
}

// ParseSuggestionsJSON parses Firefox YouTube autocomplete JSON response: [query, [10 strings], ...].
func ParseSuggestionsJSON(data []byte) ([]string, error) {
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decoding suggestions JSON: %w", err)
	}

	if len(raw) < 2 {
		return []string{}, nil
	}

	itemsList, ok := raw[1].([]any)
	if !ok {
		return []string{}, nil
	}

	suggestions := make([]string, 0, len(itemsList))
	for _, it := range itemsList {
		if s, ok := it.(string); ok && s != "" {
			suggestions = append(suggestions, s)
		}
	}

	return suggestions, nil
}

// SuggestManager coordinates debouncing, cancelling in-flight requests, and caching.
type SuggestManager struct {
	client   *SuggestionsClient
	clock    Clock
	debounce time.Duration

	mu           sync.Mutex
	currentQuery string
	timer        Timer
	cancelFunc   context.CancelFunc
	onResult     func(query string, suggestions []string)
}

// NewSuggestManager creates a manager for search box typeahead.
func NewSuggestManager(client *SuggestionsClient, onResult func(query string, suggestions []string)) *SuggestManager {
	return NewSuggestManagerWithClock(client, realClock{}, DefaultSuggestDebounce, onResult)
}

// NewSuggestManagerWithClock creates a manager with custom clock and debounce duration.
func NewSuggestManagerWithClock(client *SuggestionsClient, clock Clock, debounce time.Duration, onResult func(query string, suggestions []string)) *SuggestManager {
	if client == nil {
		client = NewSuggestionsClient(nil)
	}
	if clock == nil {
		clock = realClock{}
	}
	if debounce <= 0 {
		debounce = DefaultSuggestDebounce
	}
	return &SuggestManager{
		client:   client,
		clock:    clock,
		debounce: debounce,
		onResult: onResult,
	}
}

// Input registers a keystroke query change. It cancels any pending timer or in-flight request
// and schedules a new fetch after the debounce interval.
func (m *SuggestManager) Input(query string) {
	trimmed := strings.TrimSpace(query)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel existing timer.
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}

	// Cancel existing in-flight HTTP request.
	if m.cancelFunc != nil {
		m.cancelFunc()
		m.cancelFunc = nil
	}

	m.currentQuery = trimmed

	if IsURLQuery(trimmed) {
		if m.onResult != nil {
			m.onResult(trimmed, nil)
		}
		return
	}

	// Check if already cached.
	if m.client.Cache != nil {
		if cached, ok := m.client.Cache.Get(trimmed); ok {
			if m.onResult != nil {
				m.onResult(trimmed, cached)
			}
			return
		}
	}

	// Schedule debounced fetch.
	targetQuery := trimmed
	m.timer = m.clock.AfterFunc(m.debounce, func() {
		go func() {
			m.mu.Lock()
			if m.currentQuery != targetQuery {
				m.mu.Unlock()
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			m.cancelFunc = cancel
			m.mu.Unlock()

			suggestions, err := m.client.Fetch(ctx, targetQuery)

			m.mu.Lock()
			defer m.mu.Unlock()

			if errors.Is(ctx.Err(), context.Canceled) || m.currentQuery != targetQuery {
				return
			}
			m.cancelFunc = nil

			if err == nil && m.onResult != nil {
				m.onResult(targetQuery, suggestions)
			}
		}()
	})
}

// Cancel immediately stops any pending timer and cancels in-flight requests.
func (m *SuggestManager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if m.cancelFunc != nil {
		m.cancelFunc()
		m.cancelFunc = nil
	}
}
