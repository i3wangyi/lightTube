package ui

import (
	"context"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/embed"
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

	// 8. Test Shorts space in demo mode
	ui.switchSpace(SpaceShorts)
	frame()
	for c := 0; c < 3; c++ {
		ui.spaces.Active().Chip = c
		ui.loadShortsSpaceContent(c)
		frame()
	}
	ui.spaces.ShortsNextClick.Click()
	frame()
	ui.spaces.ShortsPrevClick.Click()
	frame()
	ui.spaces.ShortsSaveClick.Click()
	frame()
	ui.spaces.ShortsRelatedClick.Click()
	frame()
	ui.spaces.ShortsOpenWatchBtn.Click()
	frame()
	ui.switchSpace(SpaceShorts)
	ui.search("city")
	frame()

	// 9. Test Podcasts space in demo mode
	ui.switchSpace(SpacePodcasts)
	frame()
	for c := 0; c < 3; c++ {
		ui.spaces.Active().Chip = c
		ui.loadPodcastsSpaceContent(c)
		frame()
	}
	if len(ui.spaces.ShowClicks) > 0 {
		ui.spaces.ShowClicks[0].Click()
		frame()
	}
	if len(ui.spaces.EpisodeClicks) > 0 {
		ui.spaces.EpisodeClicks[0].Click()
		frame()
		if ui.state.Status != "Playback disabled in demo" {
			t.Fatalf("expected 'Playback disabled in demo' status on podcast episode click, got %q", ui.state.Status)
		}
	}
	ui.search("view")
	frame()

	// 10. Test Music space in demo mode
	ui.switchSpace(SpaceMusic)
	frame()
	for c := 0; c < 3; c++ {
		ui.spaces.Active().Chip = c
		ui.loadMusicSpaceContent(c)
		frame()
	}
	ui.spaces.MusicOpenColBtn.Click()
	frame()
	ui.spaces.MusicSaveColBtn.Click()
	frame()
	if len(ui.spaces.TrackClicks) > 0 {
		ui.spaces.TrackClicks[0].Click()
		frame()
		if ui.state.Status != "Playback disabled in demo" {
			t.Fatalf("expected 'Playback disabled in demo' status on track click, got %q", ui.state.Status)
		}
	}
	ui.spaces.MusicDockPlay.Click()
	frame()
	if ui.state.Status != "Playback disabled in demo" {
		t.Fatalf("expected 'Playback disabled in demo' status on music dock play click, got %q", ui.state.Status)
	}
	ui.search("hour")
	frame()

	// Assert zero network and exec calls were made across all spaces, chips, search, hero and card clicks
	if netCalls := netCounter.count.Load(); netCalls != 0 {
		t.Fatalf("demo mode made %d network calls, expected 0", netCalls)
	}
	if execCalls := execCounter.count.Load(); execCalls != 0 {
		t.Fatalf("demo mode made %d exec calls, expected 0", execCalls)
	}
}

// TestQueueIsolationPerSpace verifies that Watch, Shorts, Podcasts, and Music each maintain
// their own independent queue and that modifying one queue never affects another.
func TestQueueIsolationPerSpace(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}
	ui := New(Options{Store: st, Demo: true})

	// 1. Assign queue to Watch space
	ui.switchSpace(SpaceWatch)
	ui.spaces.Active().Queue = []yt.Video{
		{ID: "w1", Title: "Watch 1"},
		{ID: "w2", Title: "Watch 2"},
	}

	// 2. Assign queue to Shorts space
	ui.switchSpace(SpaceShorts)
	ui.spaces.Active().Queue = []yt.Video{
		{ID: "s1", Title: "Short 1"},
	}

	// 3. Assign queue to Podcasts space
	ui.switchSpace(SpacePodcasts)
	ui.spaces.Active().Queue = []yt.Video{
		{ID: "p1", Title: "Podcast Ep 1"},
		{ID: "p2", Title: "Podcast Ep 2"},
		{ID: "p3", Title: "Podcast Ep 3"},
	}

	// 4. Assign queue to Music space
	ui.switchSpace(SpaceMusic)
	ui.spaces.Active().Queue = []yt.Video{
		{ID: "m1", Title: "Track 1"},
		{ID: "m2", Title: "Track 2"},
	}

	// Verify all queues are isolated
	if len(ui.spaces.State(SpaceWatch).Queue) != 2 || ui.spaces.State(SpaceWatch).Queue[0].ID != "w1" {
		t.Errorf("Watch queue mismatch: %v", ui.spaces.State(SpaceWatch).Queue)
	}
	if len(ui.spaces.State(SpaceShorts).Queue) != 1 || ui.spaces.State(SpaceShorts).Queue[0].ID != "s1" {
		t.Errorf("Shorts queue mismatch: %v", ui.spaces.State(SpaceShorts).Queue)
	}
	if len(ui.spaces.State(SpacePodcasts).Queue) != 3 || ui.spaces.State(SpacePodcasts).Queue[2].ID != "p3" {
		t.Errorf("Podcasts queue mismatch: %v", ui.spaces.State(SpacePodcasts).Queue)
	}
	if len(ui.spaces.State(SpaceMusic).Queue) != 2 || ui.spaces.State(SpaceMusic).Queue[1].ID != "m2" {
		t.Errorf("Music queue mismatch: %v", ui.spaces.State(SpaceMusic).Queue)
	}

	// Mutate Music queue, verify others unchanged
	ui.spaces.State(SpaceMusic).Queue = nil
	if len(ui.spaces.State(SpaceWatch).Queue) != 2 {
		t.Errorf("Watch queue was modified when Music queue cleared")
	}
	if len(ui.spaces.State(SpacePodcasts).Queue) != 3 {
		t.Errorf("Podcasts queue was modified when Music queue cleared")
	}
}

// TestRestoringSpaceQueryAfterAllYouTubeSearch verifies that searching via "All YouTube"
// switches to Watch space and that returning to the original space restores its own query, chip, and state.
func TestRestoringSpaceQueryAfterAllYouTubeSearch(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}
	ui := New(Options{Store: st, Demo: true})

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

	// Start in Shorts space
	ui.switchSpace(SpaceShorts)
	ui.spaces.Active().Chip = 1
	ui.searchEditor.SetText("crafts")
	ui.lastSearchText = "crafts"
	ui.suggestItems = []string{"crafts diy"}
	ui.suggestVisible = true
	frame()

	// Simulate All YouTube click
	ui.allYouTubeClick.Click()
	frame()

	// Verify UI switched to Watch space and search was executed
	if ui.spaces.ActiveSpace != SpaceWatch {
		t.Fatalf("expected active space Watch after All YouTube search, got %v", ui.spaces.ActiveSpace)
	}
	if ui.searchEditor.Text() != "crafts" {
		t.Fatalf("expected search editor 'crafts' in Watch space, got %q", ui.searchEditor.Text())
	}

	// Switch back to Shorts space
	ui.switchSpace(SpaceShorts)
	frame()

	// Verify Shorts space query and chip are restored
	if ui.spaces.ActiveSpace != SpaceShorts {
		t.Fatalf("expected active space Shorts, got %v", ui.spaces.ActiveSpace)
	}
	if ui.spaces.Active().Chip != 1 {
		t.Fatalf("expected Shorts chip 1 restored, got %d", ui.spaces.Active().Chip)
	}
	if ui.searchEditor.Text() != "crafts" {
		t.Fatalf("expected Shorts query 'crafts' restored, got %q", ui.searchEditor.Text())
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

func TestDrawCoverImageHelper(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}
	ui := New(Options{Store: st})

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(1000, 1000)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
	}

	inputs := []struct {
		name string
		img  image.Image
	}{
		{"wide_200x100", image.NewRGBA(image.Rect(0, 0, 200, 100))},
		{"tall_100x200", image.NewRGBA(image.Rect(0, 0, 100, 200))},
		{"square_150x150", image.NewRGBA(image.Rect(0, 0, 150, 150))},
		{"nil_image", nil},
	}

	targets := []struct {
		name string
		pt   image.Point
	}{
		{"portrait_9_16", image.Pt(180, 320)},
		{"landscape_16_9", image.Pt(320, 180)},
		{"album_square", image.Pt(196, 196)},
		{"track_square", image.Pt(44, 44)},
	}

	for _, in := range inputs {
		for _, tgt := range targets {
			dims := ui.drawCoverImage(gtx, in.img, tgt.pt, unit.Dp(4))
			if dims.Size != tgt.pt {
				t.Errorf("drawCoverImage(%s, %s): expected size %v, got %v", in.name, tgt.name, tgt.pt, dims.Size)
			}
		}
	}
}

type fakeEmbedRecorder struct {
	rects   []image.Rectangle
	scales  []int
	hidden  []bool
	paused  bool
	stopped bool
	plays   []embed.PlayOptions
}

func (f *fakeEmbedRecorder) Play(opts embed.PlayOptions) error {
	f.plays = append(f.plays, opts)
	return nil
}
func (f *fakeEmbedRecorder) Stop() { f.stopped = true }
func (f *fakeEmbedRecorder) SetRect(rect image.Rectangle, scale int) {
	f.rects = append(f.rects, rect)
	f.scales = append(f.scales, scale)
}
func (f *fakeEmbedRecorder) SetHidden(hidden bool)               { f.hidden = append(f.hidden, hidden) }
func (f *fakeEmbedRecorder) TogglePause()                        { f.paused = !f.paused }
func (f *fakeEmbedRecorder) SetPause(p bool)                     { f.paused = p }
func (f *fakeEmbedRecorder) Seek(seconds float64, relative bool) {}
func (f *fakeEmbedRecorder) SetVolume(volume float64)            {}
func (f *fakeEmbedRecorder) ToggleMute()                         {}
func (f *fakeEmbedRecorder) Status() embed.Status                { return embed.Status{} }
func (f *fakeEmbedRecorder) Close()                              {}

func TestEmbedPlayerVideoBoxPlacement(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}
	ui := New(Options{Store: st})
	recorder := &fakeEmbedRecorder{}
	ui.embedPlayer = recorder
	ui.currentScale = 1

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1200, 760)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Now(),
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	// 1. Shorts Space (chip 0: Following - portrait stage has player)
	ui.switchSpace(SpaceShorts)
	ui.spaces.State(SpaceShorts).Chip = 0
	ui.spaces.State(SpaceShorts).Shorts = []yt.Video{{ID: "s1", Title: "Test Short 1"}}
	ui.spaces.State(SpaceShorts).ResumeItem = &ui.spaces.State(SpaceShorts).Shorts[0]
	ui.playerOwnerSpace, ui.playerOwnerID = SpaceShorts, "s1"
	frame()
	if len(recorder.rects) == 0 {
		t.Fatal("expected SetRect call on Shorts space")
	}
	shortsRect := recorder.rects[len(recorder.rects)-1]
	if shortsRect.Empty() {
		t.Fatalf("expected non-empty Shorts stage video rect, got %v", shortsRect)
	}
	if recorder.hidden[len(recorder.hidden)-1] {
		t.Fatalf("expected Shorts video box to not be hidden")
	}

	// 2. Switch to Explore chip on Shorts (unsupported card, no video box)
	ui.spaces.State(SpaceShorts).Chip = 2 // Explore chip
	frame()
	lastRect := recorder.rects[len(recorder.rects)-1]
	if !lastRect.Empty() {
		t.Fatalf("expected empty rect for Explore chip without player, got %v", lastRect)
	}
	if !recorder.hidden[len(recorder.hidden)-1] {
		t.Fatalf("expected hidden=true for page without player")
	}

	// 3. Podcasts Space (dock shows thumbnail when nothing plays; active video box when playing)
	ui.switchSpace(SpacePodcasts)
	frame()
	idlePodcastsRect := recorder.rects[len(recorder.rects)-1]
	if !idlePodcastsRect.Empty() {
		t.Fatalf("expected empty rect when nothing playing in Podcasts dock, got %v", idlePodcastsRect)
	}

	podEp := yt.Video{ID: "ep1", Title: "Podcast Ep 1"}
	ui.spaces.State(SpacePodcasts).ResumeItem = &podEp
	ui.playerOwnerSpace, ui.playerOwnerID = SpacePodcasts, podEp.ID
	frame()
	podcastsRect := recorder.rects[len(recorder.rects)-1]
	if podcastsRect.Empty() {
		t.Fatalf("expected non-empty Podcasts dock video rect when playing, got %v", podcastsRect)
	}
	if recorder.hidden[len(recorder.hidden)-1] {
		t.Fatalf("expected Podcasts video box to not be hidden when playing")
	}

	// 4. Music Space (dock shows thumbnail when nothing plays; active video box when playing)
	ui.switchSpace(SpaceMusic)
	frame()
	idleMusicRect := recorder.rects[len(recorder.rects)-1]
	if !idleMusicRect.Empty() {
		t.Fatalf("expected empty rect when nothing playing in Music compact player, got %v", idleMusicRect)
	}

	musicTrack := yt.Video{ID: "m1", Title: "Music Track 1"}
	ui.spaces.State(SpaceMusic).ResumeItem = &musicTrack
	ui.playerOwnerSpace, ui.playerOwnerID = SpaceMusic, musicTrack.ID
	frame()
	musicRect := recorder.rects[len(recorder.rects)-1]
	if musicRect.Empty() {
		t.Fatalf("expected non-empty Music compact player video rect when playing, got %v", musicRect)
	}
	if recorder.hidden[len(recorder.hidden)-1] {
		t.Fatalf("expected Music video box to not be hidden when playing")
	}

	// 5. Switch to a page without a player (e.g. Subscriptions management)
	ui.switchTab(TabSubscriptions)
	frame()
	pageWithoutPlayerRect := recorder.rects[len(recorder.rects)-1]
	if !pageWithoutPlayerRect.Empty() {
		t.Fatalf("expected empty rect after switching to Subscriptions tab, got %v", pageWithoutPlayerRect)
	}
	if !recorder.hidden[len(recorder.hidden)-1] {
		t.Fatalf("expected hidden=true after switching to Subscriptions tab")
	}
	if !recorder.paused {
		t.Fatalf("expected playback paused after switching to page without player")
	}
}

func TestDebugPlayFirstUsesSpaceSelectionHandlers(t *testing.T) {
	for _, tc := range []struct {
		space SpaceID
		item  yt.Video
	}{
		{SpaceShorts, yt.Video{ID: "short-live", Title: "Live short"}},
		{SpacePodcasts, yt.Video{ID: "episode-live", Title: "Live episode"}},
		{SpaceMusic, yt.Video{ID: "track-live", Title: "Live track"}},
	} {
		t.Run(fmt.Sprint(tc.space), func(t *testing.T) {
			st, err := store.NewDeferred(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ui := New(Options{Store: st})
			ui.embedPlayer = &fakeEmbedRecorder{}
			ui.debugPlayFirst = true
			ui.firstFramePresented = true
			ui.switchSpace(tc.space)
			switch tc.space {
			case SpaceShorts:
				ui.spaces.State(tc.space).Shorts = []yt.Video{tc.item}
			case SpacePodcasts:
				ui.spaces.State(tc.space).Episodes = []yt.Video{tc.item}
			case SpaceMusic:
				ui.spaces.State(tc.space).Tracks = []yt.Video{tc.item}
			}

			ui.maybeTriggerDebugPlayFirst()
			ui.drainActions()

			state := ui.spaces.State(tc.space)
			if state.ResumeItem == nil || state.ResumeItem.ID != tc.item.ID {
				t.Fatalf("first item was not selected: resume=%+v", state.ResumeItem)
			}
			if len(state.Queue) != 1 || state.Queue[0].ID != tc.item.ID {
				t.Fatalf("first item queue was not selected: queue=%+v", state.Queue)
			}
			if got := len(ui.embedPlayer.(*fakeEmbedRecorder).plays); got != 1 {
				t.Fatalf("player Play calls = %d, want 1", got)
			}
		})
	}
}

func TestNoDemoStringsInLiveMode(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	demoData := LoadDemoData()
	demoStrings := map[string]bool{
		"Architecture & city life":                    true,
		"MADE FOR THE LATE HOURS":                     true,
		"SAMPLE COLLECTION":                           true,
		"MADE FOR THE LATE HOURS · SAMPLE COLLECTION": true,
		"LIKED\nMUSIC":                                true,
		"MUSIC":                                       true,
		"Field Notes · Sample short":                  true,
		"A minute away from everything":               true,
		"The full story behind the frame":             true,
		"Sample short":                                true,
		"Short 1 of 1":                                true,
		"Provenance verified":                         true,
		"Shorts feed · Provenance verified":           true,
		"Sample short video preview":                  true,
		demoData.Album.Title:                          true,
		demoData.Album.Artist:                         true,
		demoData.Album.Description:                    true,
		demoData.Hero.Title:                           true,
		demoData.DockTrack.Title:                      true,
	}
	for _, trk := range demoData.Tracks {
		demoStrings[trk.Title] = true
	}
	for _, show := range demoData.Shows {
		demoStrings[show.Title] = true
	}
	for _, ep := range demoData.Episodes {
		demoStrings[ep.Title] = true
	}
	for _, sh := range demoData.Shorts {
		demoStrings[sh.Title] = true
	}

	ui := New(Options{Demo: true, Store: st})
	ui.demo, ui.demoData, ui.account, ui.thumbs = false, nil, nil, nil
	for _, space := range []string{"Watch", "", "Shorts", "Podcasts", "Music"} {
		if err := st.SaveMediaPosition(ui.currentAccountID(), "position_"+space, store.Position{
			Seconds: 42, Duration: 600, Space: space, MediaKind: "video", Title: "Stored live title", Show: "Stored live creator",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Populate live fixture items with distinct non-demo names
	ui.spaces.State(SpaceWatch).Videos = []yt.Video{
		{ID: "live_w1", Title: "Live Video 101", Channel: "Live Tech Channel"},
	}
	ui.spaces.State(SpaceShorts).Shorts = []yt.Video{
		{ID: "live_s1", Title: "Live Short 202", Channel: "Live Short Channel", URL: "https://www.youtube.com/shorts/live_s1"},
	}
	ui.spaces.State(SpacePodcasts).Shows = []yt.PodcastShow{
		{ID: "live_p1", Title: "Live Show 303", Channel: "Live Podcast Creator"},
	}
	ui.spaces.State(SpacePodcasts).Episodes = []yt.Video{
		{ID: "live_pe1", Title: "Live Episode 404", Channel: "Live Podcast Creator", Duration: 1200},
	}
	ui.spaces.State(SpaceMusic).Tracks = []yt.Video{
		{ID: "live_m1", Title: "Live Track 505", Channel: "Live Music Artist", Duration: 240},
	}

	var recordedStrings []string
	ui.labelHook = func(s string) {
		recordedStrings = append(recordedStrings, s)
	}

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1200, 760)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Now(),
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	for _, state := range ui.spaces.Spaces {
		state.ContentLoaded = true
		state.ContentGeneration = ui.currentGen()
	}
	// Keep the live feed available for the first Watch frame.
	ui.setWatchVideos(ui.spaces.State(SpaceWatch).Videos)
	// Layout every space and every chip in live mode
	for _, spaceID := range []SpaceID{SpaceWatch, SpaceShorts, SpacePodcasts, SpaceMusic} {
		ui.switchSpace(spaceID)
		cfg := GetSpaceConfig(spaceID)
		numChips := len(cfg.Chips)
		if numChips == 0 {
			numChips = 1
		}
		for c := 0; c < numChips; c++ {
			ui.spaces.Active().Chip = c
			frame()
		}
	}

	if !slices.Contains(recordedStrings, "Stored live title") {
		t.Error("live fixture did not capture the stored hero title")
	}
	// Verify no demo string appears anywhere in rendered label strings
	for _, s := range recordedStrings {
		trimmed := strings.TrimSpace(s)
		lower := strings.ToLower(trimmed)
		for _, banned := range []string{"sample", "demo", "fictional", "provenance", "view full playlist"} {
			if strings.Contains(lower, banned) {
				t.Errorf("live mode label %q contains forbidden text %q", trimmed, banned)
			}
		}
		if demoStrings[trimmed] {
			t.Errorf("demo string %q appeared in live mode layout", trimmed)
		}
		for demoStr := range demoStrings {
			if strings.Contains(trimmed, demoStr) && len(demoStr) > 4 {
				t.Errorf("demo substring %q found in live mode label string %q", demoStr, trimmed)
			}
		}
	}
}

func TestVideoMarkerPixelExactPlacement(t *testing.T) {
	sizesDP := []image.Point{
		{1440, 1080},
		{1200, 760},
		{1000, 800},
		{1051, 800}, {1049, 800},
		{601, 800}, {599, 800},
	}

	type spaceTestCase struct {
		name      string
		setup     func(ui *UI)
		isPlayer  bool
		isExplore bool
	}

	for _, scale := range []int{1, 2} {
		for _, sizeDP := range sizesDP {
			sz := image.Pt(sizeDP.X*scale, sizeDP.Y*scale)
			w, err := headless.NewWindow(sz.X, sz.Y)
			if err != nil {
				t.Fatalf("initializing headless window (%dx%d): %v", sz.X, sz.Y, err)
			}

			testCases := []spaceTestCase{
				{
					name: "Shorts",
					setup: func(ui *UI) {
						ui.switchSpace(SpaceShorts)
						ui.spaces.State(SpaceShorts).Chip = 0
						ui.spaces.State(SpaceShorts).Shorts = []yt.Video{
							{ID: "s1", Title: "Short One", URL: "https://www.youtube.com/shorts/s1"},
						}
						ui.spaces.State(SpaceShorts).ResumeItem = &ui.spaces.State(SpaceShorts).Shorts[0]
						ui.playerOwnerSpace, ui.playerOwnerID = SpaceShorts, "s1"
						ui.watchPaused = false
					},
					isPlayer: true,
				},
				{
					name: "Podcasts",
					setup: func(ui *UI) {
						ui.switchSpace(SpacePodcasts)
						ui.spaces.State(SpacePodcasts).Chip = 0
						ui.spaces.State(SpacePodcasts).Episodes = []yt.Video{
							{ID: "ep1", Title: "Episode One", Channel: "Podcaster", Duration: 1800},
						}
						ui.spaces.State(SpacePodcasts).ResumeItem = &ui.spaces.State(SpacePodcasts).Episodes[0]
						ui.playerOwnerSpace, ui.playerOwnerID = SpacePodcasts, "ep1"
						ui.watchPaused = false
					},
					isPlayer: true,
				},
				{
					name: "Music",
					setup: func(ui *UI) {
						ui.switchSpace(SpaceMusic)
						ui.spaces.State(SpaceMusic).Chip = 0
						ui.spaces.State(SpaceMusic).Tracks = []yt.Video{
							{ID: "m1", Title: "Track One", Channel: "Artist", Duration: 240},
						}
						ui.spaces.State(SpaceMusic).ResumeItem = &ui.spaces.State(SpaceMusic).Tracks[0]
						ui.playerOwnerSpace, ui.playerOwnerID = SpaceMusic, "m1"
						ui.watchPaused = false
					},
					isPlayer: true,
				},
				{
					name: "Watch",
					setup: func(ui *UI) {
						ui.openWatchPage(yt.Video{ID: "w1", Title: "Watch One", Duration: 600})
						ui.watchPaused = false
					},
					isPlayer: true,
				},
			}

			for _, tc := range testCases {
				dir := t.TempDir()
				st, err := store.NewDeferred(dir)
				if err != nil {
					t.Fatal(err)
				}
				ui := New(Options{
					Store:            st,
					DebugVideoMarker: true,
				})
				recorder := &fakeEmbedRecorder{}
				ui.embedPlayer = recorder
				ui.currentScale = scale

				tc.setup(ui)

				var ops op.Ops
				gtx := layout.Context{
					Ops:         &ops,
					Metric:      unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)},
					Constraints: layout.Exact(sz),
					Now:         time.Now(),
				}
				ui.layout(gtx)
				if err := w.Frame(&ops); err != nil {
					t.Fatalf("%s (%dx%d) frame error: %v", tc.name, sz.X, sz.Y, err)
				}

				// Capture screenshot and locate marker color #00FF01
				img := image.NewRGBA(image.Rect(0, 0, sz.X, sz.Y))
				if err := w.Screenshot(img); err != nil {
					t.Fatalf("%s (%dx%d) screenshot error: %v", tc.name, sz.X, sz.Y, err)
				}

				minX, minY, maxX, maxY := -1, -1, -1, -1
				for y := 0; y < sz.Y; y++ {
					for x := 0; x < sz.X; x++ {
						c := img.RGBAAt(x, y)
						if c.R == 0 && c.G == 255 && c.B == 1 && c.A == 255 {
							if minX == -1 || x < minX {
								minX = x
							}
							if maxX == -1 || x > maxX {
								maxX = x
							}
							if minY == -1 || y < minY {
								minY = y
							}
							if maxY == -1 || y > maxY {
								maxY = y
							}
						}
					}
				}

				if minX == -1 {
					t.Fatalf("%s (%dx%d): marker #00FF01 not found in rendered frame (spaceOrigin=%v, reg=%v)", tc.name, sz.X, sz.Y, ui.spaceOrigin, recorder.rects)
				}

				drawnRect := image.Rect(minX, minY, maxX+1, maxY+1)
				if len(recorder.rects) == 0 {
					t.Fatalf("%s (%dx%d): no rect registered with fake embed player", tc.name, sz.X, sz.Y)
				}
				registeredRect := recorder.rects[len(recorder.rects)-1]
				t.Logf("%s (%dx%d): drawn=%v registered=%v spaceOrigin=%v", tc.name, sz.X, sz.Y, drawnRect, registeredRect, ui.spaceOrigin)

				diffX := abs(drawnRect.Min.X - registeredRect.Min.X)
				diffY := abs(drawnRect.Min.Y - registeredRect.Min.Y)
				diffW := abs(drawnRect.Dx() - registeredRect.Dx())
				diffH := abs(drawnRect.Dy() - registeredRect.Dy())

				if diffX > 1 || diffY > 1 || diffW > 1 || diffH > 1 {
					t.Errorf("%s (%dx%d): drawn marker rect %v does not equal registered rect %v (±1px)",
						tc.name, sz.X, sz.Y, drawnRect, registeredRect)
				}
			}

			w.Release()
		}
	}

	// Assert rect is empty after switching to a space or chip without a player
	dir := t.TempDir()
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}
	ui := New(Options{Store: st, DebugVideoMarker: true})
	recorder := &fakeEmbedRecorder{}
	ui.embedPlayer = recorder
	ui.currentScale = 1

	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Constraints: layout.Exact(image.Pt(1200, 760)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Now(),
		}
		ui.layout(gtx)
	}

	// 1. Shorts with active player
	ui.switchSpace(SpaceShorts)
	ui.spaces.State(SpaceShorts).Chip = 0
	ui.spaces.State(SpaceShorts).Shorts = []yt.Video{{ID: "s1", Title: "Short 1"}}
	ui.spaces.State(SpaceShorts).ResumeItem = &ui.spaces.State(SpaceShorts).Shorts[0]
	ui.playerOwnerSpace, ui.playerOwnerID = SpaceShorts, "s1"
	frame()
	if recorder.rects[len(recorder.rects)-1].Empty() {
		t.Fatal("expected non-empty rect for active Shorts player")
	}

	// 2. Switch to Explore chip without player (unsupported in live mode)
	ui.spaces.State(SpaceShorts).Chip = 2
	frame()
	if !recorder.rects[len(recorder.rects)-1].Empty() {
		t.Errorf("expected empty rect for Explore chip without player, got %v", recorder.rects[len(recorder.rects)-1])
	}
	if !recorder.hidden[len(recorder.hidden)-1] {
		t.Error("expected hidden=true for Explore chip")
	}

	// 3. Switch to Subscriptions tab without player
	ui.switchTab(TabSubscriptions)
	frame()
	if !recorder.rects[len(recorder.rects)-1].Empty() {
		t.Errorf("expected empty rect after switching to Subscriptions tab, got %v", recorder.rects[len(recorder.rects)-1])
	}
	if !recorder.hidden[len(recorder.hidden)-1] {
		t.Error("expected hidden=true for Subscriptions tab")
	}

	// 4. Scroll video out of view on Watch page
	ui.openWatchPage(yt.Video{ID: "w1", Title: "Watch 1", Duration: 600})
	// In narrow window mode (< 1000dp), watchList scrolls
	partialWindow, err := headless.NewWindow(599, 800)
	if err != nil {
		t.Fatal(err)
	}
	gtxNarrow := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(599, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
	}
	ui.watchList.Position.First = 0
	ui.watchList.Position.Offset = 40
	ops.Reset()
	ui.layout(gtxNarrow)
	if !recorder.rects[len(recorder.rects)-1].Empty() {
		t.Errorf("expected empty rect when scroll clips video, got %v", recorder.rects[len(recorder.rects)-1])
	}
	if err := partialWindow.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	partialImage := image.NewRGBA(image.Rect(0, 0, 599, 800))
	if err := partialWindow.Screenshot(partialImage); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < partialImage.Bounds().Dy(); y++ {
		for x := 0; x < partialImage.Bounds().Dx(); x++ {
			c := partialImage.RGBAAt(x, y)
			if c.R == 0 && c.G == 255 && c.B == 1 && c.A == 255 {
				t.Fatalf("video marker remains visible after partial scroll at (%d,%d)", x, y)
			}
		}
	}
	partialWindow.Release()

	ui.watchList.Position.First = 1
	ui.watchList.Position.Offset = 1000
	ops.Reset()
	ui.layout(gtxNarrow)

	if !recorder.rects[len(recorder.rects)-1].Empty() {
		t.Errorf("expected empty rect after scrolling video out of view, got %v", recorder.rects[len(recorder.rects)-1])
	}
	if !recorder.hidden[len(recorder.hidden)-1] {
		t.Error("expected hidden=true after scrolling video out of view")
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
