package ui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

func TestAccountUIInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "cookies.txt")
	cacheDir := filepath.Join(tmpDir, "cache")
	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		Browser:    "testbrowser+keyring",
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
	})

	ui := New(Options{
		Account: acc,
	})

	if ui.account != acc {
		t.Errorf("expected ui.account to match provided account instance")
	}
	if text := ui.accountBrowserEditor.Text(); text != "testbrowser+keyring" {
		t.Errorf("expected browser editor text %q, got %q", "testbrowser+keyring", text)
	}
	if len(ui.guideClicks) != 5 {
		t.Errorf("expected 5 guide clicks, got %d", len(ui.guideClicks))
	}
}

func TestAccountCachedFeedsWhenSignedIn(t *testing.T) {
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "cookies.txt")
	_ = os.WriteFile(cookieFile, []byte("test-cookie-content"), 0o600)

	cacheDir := filepath.Join(tmpDir, "cache")
	_ = os.MkdirAll(cacheDir, 0o700)

	// Seed cached feeds
	homeVideos := []yt.Video{{ID: "h1", Title: "Home Vid 1"}}
	subsVideos := []yt.Video{{ID: "s1", Title: "Subs Vid 1"}}
	laterVideos := []yt.Video{{ID: "l1", Title: "Later Vid 1"}}
	histVideos := []yt.Video{{ID: "hist1", Title: "History Vid 1"}}

	_ = store.SaveAccountFeedWithDir(cacheDir, "home", homeVideos)
	_ = store.SaveAccountFeedWithDir(cacheDir, "subs", subsVideos)
	_ = store.SaveAccountFeedWithDir(cacheDir, "later", laterVideos)
	_ = store.SaveAccountFeedWithDir(cacheDir, "history", histVideos)

	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: "/bin/false",
	})

	ui := New(Options{
		Account: acc,
	})

	// 1. TabHome
	ui.switchTab(TabHome)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "h1" {
		t.Errorf("expected cached home video h1, got %+v", ui.state.Videos)
	}

	// 2. TabSubscriptions
	ui.switchTab(TabSubscriptions)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "s1" {
		t.Errorf("expected cached subs video s1, got %+v", ui.state.Videos)
	}

	// 3. TabWatchLater
	ui.switchTab(TabWatchLater)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "l1" {
		t.Errorf("expected cached later video l1, got %+v", ui.state.Videos)
	}

	// 4. TabHistory
	ui.switchTab(TabHistory)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "hist1" {
		t.Errorf("expected cached history video hist1, got %+v", ui.state.Videos)
	}
}

func TestAccountFeedsWhenSignedOut(t *testing.T) {
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "nonexistent-cookies.txt")
	cacheDir := filepath.Join(tmpDir, "cache")

	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: "/bin/false",
	})

	st, err := store.NewDeferred(tmpDir)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	_, _ = st.ToggleWatchLater(yt.Video{ID: "local-later", Title: "Local Later"})
	_ = st.AddHistory(yt.Video{ID: "local-hist", Title: "Local History"})

	ui := New(Options{
		Account: acc,
		Store:   st,
	})

	// Watch later should use local store
	ui.switchTab(TabWatchLater)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "local-later" {
		t.Errorf("expected local watch later video, got %+v", ui.state.Videos)
	}

	// History should use local store
	ui.switchTab(TabHistory)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "local-hist" {
		t.Errorf("expected local history video, got %+v", ui.state.Videos)
	}

	// Subs should have empty videos (manage mode)
	ui.switchTab(TabSubscriptions)
	if len(ui.state.Videos) != 0 {
		t.Errorf("expected 0 videos on signed out subs tab, got %d", len(ui.state.Videos))
	}
}

func TestDrawPersonIcon(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Constraints: layout.Constraints{
			Min: image.Pt(48, 48),
			Max: image.Pt(48, 48),
		},
		Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2},
	}

	bounds := image.Rect(0, 0, 48, 48)
	col := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	// Ensure vector drawing does not panic
	drawPersonIcon(gtx, bounds, col)
}

func TestRefreshAccountFeedGating(t *testing.T) {
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "cookies.txt")
	_ = os.WriteFile(cookieFile, []byte("cookies"), 0o600)
	cacheDir := filepath.Join(tmpDir, "cache")

	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		CookieFile: cookieFile,
		CacheDir:   cacheDir,
		BinaryPath: "/bin/sleep",
	})

	ui := New(Options{
		Account: acc,
	})

	// First refresh marks it in-flight
	ui.refreshAccountFeed(yt.FeedHome)

	ui.refreshMu.Lock()
	inFlight := ui.refreshingFeeds[yt.FeedHome]
	ui.refreshMu.Unlock()

	if !inFlight {
		t.Errorf("expected FeedHome to be in-flight")
	}

	// Second concurrent call should not launch duplicate or panic
	ui.refreshAccountFeed(yt.FeedHome)

	// Wait for background goroutine to complete before TempDir cleanup
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		ui.refreshMu.Lock()
		active := ui.refreshingFeeds[yt.FeedHome]
		ui.refreshMu.Unlock()
		if !active {
			break
		}
	}
}

func TestLayoutAccountView(t *testing.T) {
	tmpDir := t.TempDir()
	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		CookieFile: filepath.Join(tmpDir, "nonexistent.txt"),
		CacheDir:   tmpDir,
	})

	ui := New(Options{
		Account: acc,
	})

	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Constraints: layout.Constraints{
			Min: image.Pt(800, 600),
			Max: image.Pt(800, 600),
		},
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	// 1. Signed out layout
	dims := ui.layoutAccountView(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("expected positive dimensions, got %v", dims.Size)
	}

	// 2. Signing in state layout
	ui.accountSigningIn = true
	dims = ui.layoutAccountView(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("expected positive dimensions in signing in state, got %v", dims.Size)
	}
	ui.accountSigningIn = false

	// 3. Error state layout
	ui.accountError = "Login failed: invalid keyring"
	dims = ui.layoutAccountView(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("expected positive dimensions in error state, got %v", dims.Size)
	}

	// 4. Signed in state layout
	_ = os.WriteFile(acc.CookieFile, []byte("cookies"), 0o600)
	ui.accountError = ""
	dims = ui.layoutAccountView(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("expected positive dimensions in signed in state, got %v", dims.Size)
	}
}

func TestLayoutSubscriptionsViewSignedIn(t *testing.T) {
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "cookies.txt")
	_ = os.WriteFile(cookieFile, []byte("cookies"), 0o600)

	acc := yt.NewAccountWithOptions(yt.AccountOptions{
		CookieFile: cookieFile,
		CacheDir:   tmpDir,
	})

	st, err := store.NewDeferred(tmpDir)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	_ = st.AddSubscription(yt.Subscription{ChannelID: "c1", Title: "Channel 1"})

	ui := New(Options{
		Account: acc,
		Store:   st,
	})
	ui.state.Subscriptions = st.Subscriptions()
	ui.state.SetVideos([]yt.Video{
		{ID: "v1", Title: "Sub Video 1", Channel: "Channel 1", ChannelID: "c1"},
		{ID: "v2", Title: "Sub Video 2", Channel: "Channel 1", ChannelID: "c1"},
	})

	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Constraints: layout.Constraints{
			Min: image.Pt(800, 600),
			Max: image.Pt(800, 600),
		},
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutSubscriptionsView(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("expected positive dimensions, got %v", dims.Size)
	}
}
