package yt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseSuggestionsJSON(t *testing.T) {
	raw := []byte(`["golang",["golang","golang tutorial","golang project","golang course"],[],{}]`)
	items, err := ParseSuggestionsJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing suggestions: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	if items[0] != "golang" || items[1] != "golang tutorial" {
		t.Errorf("unexpected items: %v", items)
	}
}

func TestSuggestionsClientWithHTTPTest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q != "arch" {
			t.Errorf("expected query arch, got %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`["arch",["arch linux","arch install","arch wiki"]]`))
	}))
	defer ts.Close()

	client := NewSuggestionsClient(ts.Client())
	client.BaseURL = ts.URL + "/complete/search?client=firefox&ds=yt&q="

	results, err := client.Fetch(context.Background(), "arch")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0] != "arch linux" || results[1] != "arch install" || results[2] != "arch wiki" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestSuggestionsCacheLRU(t *testing.T) {
	cache := NewSuggestionsCache(3)

	cache.Put("a", []string{"a1"})
	cache.Put("b", []string{"b1"})
	cache.Put("c", []string{"c1"})

	if cache.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", cache.Len())
	}

	// Access "a" so "b" becomes oldest.
	if _, ok := cache.Get("a"); !ok {
		t.Fatal("expected 'a' to be present")
	}

	// Insert "d" -> "b" should be evicted.
	cache.Put("d", []string{"d1"})

	if _, ok := cache.Get("b"); ok {
		t.Fatal("expected 'b' to be evicted")
	}
	if _, ok := cache.Get("a"); !ok {
		t.Fatal("expected 'a' to still be present")
	}
	if _, ok := cache.Get("c"); !ok {
		t.Fatal("expected 'c' to still be present")
	}
	if _, ok := cache.Get("d"); !ok {
		t.Fatal("expected 'd' to be present")
	}
}

func TestSuggestionsClientCacheHitSkipsHTTP(t *testing.T) {
	var requestCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`["test",["test 1","test 2"]]`))
	}))
	defer ts.Close()

	client := NewSuggestionsClient(ts.Client())
	client.BaseURL = ts.URL + "/complete/search?client=firefox&ds=yt&q="

	// First fetch: hits server
	res1, err := client.Fetch(context.Background(), "test")
	if err != nil || len(res1) != 2 {
		t.Fatalf("res1 error: %v, len: %d", err, len(res1))
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount)
	}

	// Second fetch: served from cache
	res2, err := client.Fetch(context.Background(), "test")
	if err != nil || len(res2) != 2 {
		t.Fatalf("res2 error: %v, len: %d", err, len(res2))
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected still 1 request, got %d (cache miss!)", requestCount)
	}
}

func TestIsURLQuery(t *testing.T) {
	cases := []struct {
		input string
		isURL bool
	}{
		{"", true},
		{"   ", true},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"http://youtu.be/dQw4w9WgXcQ", true},
		{"dQw4w9WgXcQ", true}, // 11-char ID
		{"golang", false},
		{"golang tutorial", false},
	}

	for _, c := range cases {
		got := IsURLQuery(c.input)
		if got != c.isURL {
			t.Errorf("IsURLQuery(%q) = %v, want %v", c.input, got, c.isURL)
		}
	}
}

// FakeClock implements Clock without time.Sleep.
type fakeTimer struct {
	clock   *fakeClock
	f       func()
	stopped bool
}

func (ft *fakeTimer) Stop() bool {
	ft.clock.mu.Lock()
	defer ft.clock.mu.Unlock()
	if ft.stopped {
		return false
	}
	ft.stopped = true
	return true
}

type fakeClock struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func newFakeClock() *fakeClock {
	return &fakeClock{}
}

func (fc *fakeClock) Now() time.Time {
	return time.Time{}
}

func (fc *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	ft := &fakeTimer{clock: fc, f: f}
	fc.timers = append(fc.timers, ft)
	return ft
}

func (fc *fakeClock) TriggerAll() {
	fc.mu.Lock()
	active := make([]func(), 0, len(fc.timers))
	for _, t := range fc.timers {
		if !t.stopped {
			active = append(active, t.f)
			t.stopped = true
		}
	}
	fc.timers = nil
	fc.mu.Unlock()

	for _, f := range active {
		f()
	}
}

func (fc *fakeClock) ActiveCount() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	count := 0
	for _, t := range fc.timers {
		if !t.stopped {
			count++
		}
	}
	return count
}

func TestSuggestManagerDebounceWithFakeClock(t *testing.T) {
	clock := newFakeClock()

	var queriesReceived []string
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mu.Lock()
		queriesReceived = append(queriesReceived, q)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`[%q,[%q]]`, q, q+"-result")))
	}))
	defer ts.Close()

	client := NewSuggestionsClient(ts.Client())
	client.BaseURL = ts.URL + "/complete/search?client=firefox&ds=yt&q="

	resultsChan := make(chan []string, 5)
	mgr := NewSuggestManagerWithClock(client, clock, 150*time.Millisecond, func(query string, suggestions []string) {
		resultsChan <- suggestions
	})

	// User types "g", then "go", then "gol" rapidly before debounce fires.
	mgr.Input("g")
	mgr.Input("go")
	mgr.Input("gol")

	// Verify that 2 earlier timers were stopped and only 1 timer is active.
	if clock.ActiveCount() != 1 {
		t.Fatalf("expected 1 active timer, got %d", clock.ActiveCount())
	}

	// Trigger the debounce timer via fake clock (no sleep).
	clock.TriggerAll()

	// Wait for results
	select {
	case res := <-resultsChan:
		if len(res) != 1 || res[0] != "gol-result" {
			t.Fatalf("unexpected result: %v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for suggest result")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(queriesReceived) != 1 || queriesReceived[0] != "gol" {
		t.Fatalf("expected only query 'gol' to be sent, got %v", queriesReceived)
	}
}

func TestSuggestManagerCancelInFlight(t *testing.T) {
	clock := newFakeClock()

	fetchStarted := make(chan struct{})
	allowFetchToProceed := make(chan struct{})
	contextCancelled := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(fetchStarted)
		select {
		case <-r.Context().Done():
			close(contextCancelled)
			return
		case <-allowFetchToProceed:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`["a",["a-res"]]`))
	}))
	defer ts.Close()

	client := NewSuggestionsClient(ts.Client())
	client.BaseURL = ts.URL + "/complete/search?client=firefox&ds=yt&q="

	mgr := NewSuggestManagerWithClock(client, clock, 150*time.Millisecond, func(query string, suggestions []string) {})

	// Start request "a"
	mgr.Input("a")
	clock.TriggerAll()

	// Wait until the HTTP request has started on the server
	<-fetchStarted

	// Now user cancels or enters a new input "b"
	mgr.Input("b")

	// The in-flight context for "a" must be cancelled
	select {
	case <-contextCancelled:
		// Success!
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight request to be cancelled")
	}
}
