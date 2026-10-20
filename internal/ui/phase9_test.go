package ui

import (
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

type testFakeClock struct {
	mu     sync.Mutex
	timers []*testFakeTimer
}

type testFakeTimer struct {
	clock   *testFakeClock
	f       func()
	stopped bool
}

func (ft *testFakeTimer) Stop() bool {
	ft.clock.mu.Lock()
	defer ft.clock.mu.Unlock()
	if ft.stopped {
		return false
	}
	ft.stopped = true
	return true
}

func (fc *testFakeClock) Now() time.Time {
	return time.Time{}
}

func (fc *testFakeClock) AfterFunc(d time.Duration, f func()) yt.Timer {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	ft := &testFakeTimer{clock: fc, f: f}
	fc.timers = append(fc.timers, ft)
	return ft
}

func (fc *testFakeClock) TriggerAll() {
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

func TestTypeaheadHeadless(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		if q == "golang" {
			_, _ = w.Write([]byte(`["golang",["golang tutorial","golang concurrency","golang generics"]]`))
		} else {
			_, _ = w.Write([]byte(`["",[]]`))
		}
	}))
	defer ts.Close()

	suggestClient := yt.NewSuggestionsClient(ts.Client())
	suggestClient.BaseURL = ts.URL + "/complete/search?client=firefox&ds=yt&q="

	clock := &testFakeClock{}
	queries := make(chan string, 1)

	ui := New(Options{
		YTClient:      yt.NewClientWithOptions(fakeYTDLP{queries}, http.DefaultClient, dir),
		Store:         st,
		Account:       yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
		SuggestClient: suggestClient,
		SuggestClock:  clock,
	})

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	frame()

	// Focus search editor and type "golang"
	ui.searchEditor.SetText("golang")
	r.Source().Execute(key.FocusCmd{Tag: &ui.searchEditor})
	frame()

	// Trigger debounce timer on clock
	clock.TriggerAll()

	// Allow goroutine to finish fetch and register action
	time.Sleep(50 * time.Millisecond)
	frame()

	if !ui.suggestVisible {
		t.Fatalf("expected suggestions to be visible, got false")
	}
	if len(ui.suggestItems) != 3 {
		t.Fatalf("expected 3 suggestions, got %d (%v)", len(ui.suggestItems), ui.suggestItems)
	}

	// Down arrow selects first item ("golang tutorial")
	r.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	frame()
	if ui.suggestSelected != 0 {
		t.Fatalf("expected selected index 0, got %d", ui.suggestSelected)
	}

	// Down arrow selects second item ("golang concurrency")
	r.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	frame()
	if ui.suggestSelected != 1 {
		t.Fatalf("expected selected index 1, got %d", ui.suggestSelected)
	}

	// Enter searches the selected second suggestion
	r.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	frame()

	select {
	case q := <-queries:
		if q != "golang concurrency" {
			t.Fatalf("expected search query 'golang concurrency', got %q", q)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("search was not triggered on Enter")
	}

	if ui.suggestVisible {
		t.Fatalf("expected suggestions to be closed after Enter")
	}

	// Test Esc closes dropdown
	ui.searchEditor.SetText("golang")
	frame()
	clock.TriggerAll()
	time.Sleep(50 * time.Millisecond)
	frame()

	if !ui.suggestVisible {
		t.Fatalf("expected suggestions visible again")
	}

	// Send Escape
	r.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()

	if ui.suggestVisible {
		t.Fatalf("expected Escape to close suggestions dropdown")
	}
}

func TestAccountSwitchReloadsFeeds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	cache1 := dir + "/cache1"
	cache2 := dir + "/cache2"
	_ = os.MkdirAll(cache1, 0700)
	_ = os.MkdirAll(cache2, 0700)

	v1 := []yt.Video{{ID: "vid1", Title: "Account 1 Home Video"}}
	v2 := []yt.Video{{ID: "vid2", Title: "Account 2 Home Video"}}
	data1, _ := json.Marshal(v1)
	data2, _ := json.Marshal(v2)
	_ = os.WriteFile(cache1+"/home.json", data1, 0600)
	_ = os.WriteFile(cache2+"/home.json", data2, 0600)

	cookie1 := dir + "/cookie1.txt"
	cookie2 := dir + "/cookie2.txt"
	_ = os.WriteFile(cookie1, []byte("# Netscape HTTP Cookie File\n"), 0600)
	_ = os.WriteFile(cookie2, []byte("# Netscape HTTP Cookie File\n"), 0600)

	acc1 := store.AccountConfig{
		ID:          "acc1",
		Label:       "Account 1",
		CookieFile:  cookie1,
		CacheDir:    cache1,
		BrowserSpec: "chrome",
	}
	acc2 := store.AccountConfig{
		ID:          "acc2",
		Label:       "Account 2",
		CookieFile:  cookie2,
		CacheDir:    cache2,
		BrowserSpec: "brave",
	}
	_ = st.AddAccount(acc1)
	_ = st.AddAccount(acc2)
	_, _ = st.SwitchAccount("acc1")

	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		CookieFile: cookie1,
		CacheDir:   cache1,
		Browser:    "chrome",
	})

	ui := New(Options{
		Store:   st,
		Account: acc,
	})

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	ui.switchTab(TabHome)
	frame()

	if len(ui.state.Videos) == 0 || ui.state.Videos[0].ID != "vid1" {
		t.Fatalf("expected account 1 videos, got %v", ui.state.Videos)
	}

	ui.switchAccount(acc2)
	frame()

	if ui.settings.ActiveAccount != "acc2" {
		t.Fatalf("expected active account acc2, got %s", ui.settings.ActiveAccount)
	}
	if ui.account.CookieFile != cookie2 {
		t.Fatalf("expected account cookie file %s, got %s", cookie2, ui.account.CookieFile)
	}
	if len(ui.state.Videos) == 0 || ui.state.Videos[0].ID != "vid2" {
		t.Fatalf("expected reloaded feed with account 2 videos, got %v", ui.state.Videos)
	}
}
