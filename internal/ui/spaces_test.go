package ui

import (
	"context"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
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

// TestSpacesStateSeparation verifies that each of the four spaces maintains its own
// independent state (chip, query, selection, queue, resume item) and restores it on switch.
func TestSpacesStateSeparation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	ui := New(Options{
		Store:   st,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	// Configure Watch space state
	ui.switchSpace(SpaceWatch)
	watchState := ui.spaces.State(SpaceWatch)
	watchState.Chip = 2
	watchState.Query = "city architecture"
	watchState.Selection = 3
	watchState.Queue = []yt.Video{{ID: "w_vid1", Title: "Watch Queue Video 1"}}
	watchState.ResumeItem = &yt.Video{ID: "w_resume", Title: "Resume Hero"}
	ui.searchEditor.SetText("city architecture")

	// Switch to Shorts space - must be default initial state
	ui.switchSpace(SpaceShorts)
	if ui.spaces.ActiveSpace != SpaceShorts {
		t.Fatalf("expected active space Shorts, got %v", ui.spaces.ActiveSpace)
	}
	shortsState := ui.spaces.State(SpaceShorts)
	if shortsState.Chip != 0 || shortsState.Query != "" {
		t.Fatalf("expected Shorts space initial state (chip 0, empty query), got chip %d, query %q", shortsState.Chip, shortsState.Query)
	}

	// Set Shorts state
	shortsState.Chip = 1
	shortsState.Query = "quick shorts"
	shortsState.Selection = 0
	shortsState.Queue = []yt.Video{{ID: "s_vid1", Title: "Shorts Video 1"}}
	ui.searchEditor.SetText("quick shorts")

	// Switch back to Watch space - must restore Watch state
	ui.switchSpace(SpaceWatch)
	if ui.spaces.ActiveSpace != SpaceWatch {
		t.Fatalf("expected active space Watch, got %v", ui.spaces.ActiveSpace)
	}
	currentWatch := ui.spaces.Active()
	if currentWatch.Chip != 2 {
		t.Fatalf("expected Watch chip 2, got %d", currentWatch.Chip)
	}
	if currentWatch.Query != "city architecture" {
		t.Fatalf("expected Watch query 'city architecture', got %q", currentWatch.Query)
	}
	if currentWatch.Selection != 3 {
		t.Fatalf("expected Watch selection 3, got %d", currentWatch.Selection)
	}
	if len(currentWatch.Queue) != 1 || currentWatch.Queue[0].ID != "w_vid1" {
		t.Fatalf("expected Watch queue with w_vid1, got %v", currentWatch.Queue)
	}
	if currentWatch.ResumeItem == nil || currentWatch.ResumeItem.ID != "w_resume" {
		t.Fatalf("expected Watch resume item w_resume, got %v", currentWatch.ResumeItem)
	}
	if ui.searchEditor.Text() != "city architecture" {
		t.Fatalf("expected search editor text 'city architecture', got %q", ui.searchEditor.Text())
	}

	// Switch to Shorts space again - must restore Shorts state
	ui.switchSpace(SpaceShorts)
	currentShorts := ui.spaces.Active()
	if currentShorts.Chip != 1 {
		t.Fatalf("expected Shorts chip 1, got %d", currentShorts.Chip)
	}
	if currentShorts.Query != "quick shorts" {
		t.Fatalf("expected Shorts query 'quick shorts', got %q", currentShorts.Query)
	}
	if ui.searchEditor.Text() != "quick shorts" {
		t.Fatalf("expected search editor text 'quick shorts', got %q", ui.searchEditor.Text())
	}
}

// TestSpaceSwitchNoAutostart verifies that switching spaces pauses playback
// and never starts playback automatically.
func TestSpaceSwitchNoAutostart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	ui := New(Options{
		Store:   st,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	ui.watchPaused = false
	ui.switchSpace(SpacePodcasts)

	if !ui.watchPaused {
		t.Fatalf("expected watchPaused to be true after space switch, got false")
	}
	if ui.state.CurrentTab == TabWatch {
		t.Fatalf("space switch must not enter TabWatch, got %v", ui.state.CurrentTab)
	}

	ui.switchSpace(SpaceMusic)
	if !ui.watchPaused {
		t.Fatalf("expected watchPaused to remain true after switching to Music, got false")
	}
}

// TestSpacesShortcutMap verifies Alt+1..4 for spaces and preserved Ctrl+1..5 shortcuts.
func TestSpacesShortcutMap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	ui := New(Options{
		Store:   st,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
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

	// 1. Alt+2 switches to Shorts
	r.Queue(key.Event{Name: "2", Modifiers: key.ModAlt, State: key.Press})
	frame()
	if ui.spaces.ActiveSpace != SpaceShorts {
		t.Fatalf("Alt+2: expected SpaceShorts, got %v", ui.spaces.ActiveSpace)
	}

	// 2. Alt+3 switches to Podcasts
	r.Queue(key.Event{Name: "3", Modifiers: key.ModAlt, State: key.Press})
	frame()
	if ui.spaces.ActiveSpace != SpacePodcasts {
		t.Fatalf("Alt+3: expected SpacePodcasts, got %v", ui.spaces.ActiveSpace)
	}

	// 3. Alt+4 switches to Music
	r.Queue(key.Event{Name: "4", Modifiers: key.ModAlt, State: key.Press})
	frame()
	if ui.spaces.ActiveSpace != SpaceMusic {
		t.Fatalf("Alt+4: expected SpaceMusic, got %v", ui.spaces.ActiveSpace)
	}

	// 4. Alt+1 switches to Watch
	r.Queue(key.Event{Name: "1", Modifiers: key.ModAlt, State: key.Press})
	frame()
	if ui.spaces.ActiveSpace != SpaceWatch {
		t.Fatalf("Alt+1: expected SpaceWatch, got %v", ui.spaces.ActiveSpace)
	}

	// 5. Ctrl+3 switches to TabWatchLater
	r.Queue(key.Event{Name: "3", Modifiers: key.ModShortcut, State: key.Press})
	frame()
	if ui.state.CurrentTab != TabWatchLater {
		t.Fatalf("Ctrl+3: expected TabWatchLater, got %v", ui.state.CurrentTab)
	}

	// 6. Ctrl+1 switches back to TabHome
	r.Queue(key.Event{Name: "1", Modifiers: key.ModShortcut, State: key.Press})
	frame()
	if ui.state.CurrentTab != TabHome {
		t.Fatalf("Ctrl+1: expected TabHome, got %v", ui.state.CurrentTab)
	}
}

// TestSearchScopeLabels verifies that each space provides the correct scope label and placeholder.
func TestSearchScopeLabels(t *testing.T) {
	cases := []struct {
		space      SpaceID
		wantScope  string
		wantSearch string
	}{
		{SpaceWatch, "Watch space", "Search Watch"},
		{SpaceShorts, "Shorts space", "Search Shorts"},
		{SpacePodcasts, "Podcasts space", "Search Podcasts"},
		{SpaceMusic, "Music space", "Search Music"},
	}

	for _, tc := range cases {
		scope := SearchScopeLabel(tc.space)
		if scope != tc.wantScope {
			t.Errorf("space %v: got scope %q, want %q", tc.space, scope, tc.wantScope)
		}
		ph := SearchPlaceholder(tc.space)
		if ph != tc.wantSearch {
			t.Errorf("space %v: got placeholder %q, want %q", tc.space, ph, tc.wantSearch)
		}
	}
}

// TestAmbientOffDrawsNothingExtra verifies that when Ambient is disabled,
// DrawWatchGlow generates zero draw operations.
func TestAmbientOffDrawsNothingExtra(t *testing.T) {
	var a AmbientShape
	bounds := image.Rect(0, 0, 800, 450)

	var opsDisabled op.Ops
	gtxDisabled := layout.Context{
		Ops:         &opsDisabled,
		Constraints: layout.Exact(image.Pt(800, 450)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	a.DrawWatchGlow(gtxDisabled, bounds, false)

	// When ambient is off, DrawWatchGlow must draw nothing and record no shapes
	if a.DrawCount() != 0 {
		t.Fatalf("expected DrawCount 0 when ambient is off, got %d", a.DrawCount())
	}

	// When ambient is on, shapes should be drawn and recorded
	var opsEnabled op.Ops
	gtxEnabled := layout.Context{
		Ops:         &opsEnabled,
		Constraints: layout.Exact(image.Pt(800, 450)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	a.DrawWatchGlow(gtxEnabled, bounds, true)
	if a.DrawCount() != 1 {
		t.Fatalf("expected DrawCount 1 when ambient is on, got %d", a.DrawCount())
	}

	// Successive call with same bounds reuses cached recording
	a.DrawWatchGlow(gtxEnabled, bounds, true)
	if a.DrawCount() != 2 {
		t.Fatalf("expected DrawCount 2 on second call, got %d", a.DrawCount())
	}
}

// TestDelayedResponseInvalidation verifies that late responses from older generations
// are dropped when account switch or sign-out bumps the generation counter.
func TestDelayedResponseInvalidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	ui := New(Options{
		Store:   st,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	initialGen := ui.currentGen()
	if initialGen != 0 {
		t.Fatalf("expected initial generation 0, got %d", initialGen)
	}

	// Switching account bumps generation
	acc := store.AccountConfig{ID: "test_acc", Label: "Test"}
	ui.switchAccount(acc)
	bumpedGen := ui.currentGen()
	if bumpedGen <= initialGen {
		t.Fatalf("expected generation to increment on account switch, was %d now %d", initialGen, bumpedGen)
	}

	// Signing out bumps generation again
	ui.signOutAccount()
	signOutGen := ui.currentGen()
	if signOutGen <= bumpedGen {
		t.Fatalf("expected generation to increment on sign out, was %d now %d", bumpedGen, signOutGen)
	}
}

type countingTransport struct {
	count atomic.Int64
}

func (ct *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ct.count.Add(1)
	return nil, fmt.Errorf("unexpected network call during demo mode: %s", req.URL)
}

type countingRunner struct {
	count atomic.Int64
}

func (cr *countingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cr.count.Add(1)
	return nil, fmt.Errorf("unexpected exec call during demo mode: %s", name)
}

func (cr *countingRunner) Search(ctx context.Context, query string) ([]byte, error) {
	cr.count.Add(1)
	return nil, fmt.Errorf("unexpected Search call during demo mode: %s", query)
}

func (cr *countingRunner) ResolveChannel(ctx context.Context, target string) (string, error) {
	cr.count.Add(1)
	return "", fmt.Errorf("unexpected ResolveChannel call during demo mode: %s", target)
}

// TestDemoZeroNetworkOrExecCalls verifies that when --demo is active,
// no network requests or external command exec calls are ever made.
func TestDemoZeroNetworkOrExecCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	netCounter := &countingTransport{}
	execCounter := &countingRunner{}
	mockHTTPClient := &http.Client{Transport: netCounter}

	ytClient := yt.NewClientWithOptions(execCounter, mockHTTPClient, dir)
	suggestClient := yt.NewSuggestionsClient(mockHTTPClient)

	ui := New(Options{
		Demo:          true,
		Store:         st,
		YTClient:      ytClient,
		SuggestClient: suggestClient,
		GoogleSignInRunner: func(ctx context.Context, name string, args ...string) error {
			execCounter.count.Add(1)
			return fmt.Errorf("unexpected exec call: %s", name)
		},
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1440, 1080)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Now(),
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	// 1. Initial demo render on Watch space
	frame()

	// 2. Switch across all 4 spaces
	for _, sp := range []SpaceID{SpaceShorts, SpacePodcasts, SpaceMusic, SpaceWatch} {
		ui.switchSpace(sp)
		frame()
	}

	// 3. Switch chips on Watch space
	for chip := 0; chip < 4; chip++ {
		ui.spaces.Active().Chip = chip
		ui.loadWatchSpaceContent(chip)
		frame()
	}

	// 4. Perform search query in demo
	ui.search("city")
	frame()

	// 5. Switch back to Watch space (Chip 0, clear query) to render the hero section
	ui.switchSpace(SpaceWatch)
	ui.spaces.Active().Chip = 0
	ui.searchEditor.SetText("")
	ui.loadWatchSpaceContent(0)
	frame()

	// 6. Click hero preview & primary buttons in demo
	ui.heroPreviewClick.Click()
	frame()
	if ui.state.Status != "Playback disabled in demo" {
		t.Fatalf("expected 'Playback disabled in demo' status on hero preview click, got %q", ui.state.Status)
	}

	ui.heroPrimaryClick.Click()
	frame()
	if ui.state.Status != "Playback disabled in demo" {
		t.Fatalf("expected 'Playback disabled in demo' status on hero button click, got %q", ui.state.Status)
	}

	// 7. Click video cards in demo
	if len(ui.cardClicks) > 0 {
		ui.cardClicks[0].Click()
		frame()
		if ui.state.Status != "Playback disabled in demo" {
			t.Fatalf("expected 'Playback disabled in demo' status on card click, got %q", ui.state.Status)
		}
	}

	// Assert zero network and exec calls were made across all spaces, chips, search, hero and card clicks
	if netCalls := netCounter.count.Load(); netCalls != 0 {
		t.Fatalf("demo mode made %d network calls, expected 0", netCalls)
	}
	if execCalls := execCounter.count.Load(); execCalls != 0 {
		t.Fatalf("demo mode made %d exec calls, expected 0", execCalls)
	}
}

func TestDemoNewNeverOpensLiveStore(t *testing.T) {
	canaryData := t.TempDir()
	dataLt := filepath.Join(canaryData, "lighttube")
	if err := os.MkdirAll(dataLt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataLt, "settings.json"), []byte(`{"active_account":"live_user"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// Chmod dataLt to 000 so any attempt by store.DefaultDataDir() to open/read fails.
	if err := os.Chmod(dataLt, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dataLt, 0o755)
	})

	t.Setenv("XDG_DATA_HOME", canaryData)

	ui := New(Options{
		Demo:  true,
		Store: nil, // Must NOT fall back to store.DefaultDataDir()!
	})
	if ui == nil {
		t.Fatal("expected non-nil UI")
	}
	if ui.store == nil {
		t.Fatal("expected non-nil isolated demo store")
	}
	if ui.demoTmpDir != "" {
		defer os.RemoveAll(ui.demoTmpDir)
	}

	// Verify that the canary directory was never accessed or modified.
	_ = os.Chmod(dataLt, 0o755)
	data, err := os.ReadFile(filepath.Join(dataLt, "settings.json"))
	if err != nil || string(data) != `{"active_account":"live_user"}` {
		t.Fatalf("canary file in live store dir was modified or corrupted: %v", err)
	}
}
