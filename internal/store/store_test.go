package store

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"lighttube/internal/yt"
)

func TestStoreSubscriptions(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	if len(s.Subscriptions()) != 0 {
		t.Fatalf("expected 0 initial subscriptions, got %d", len(s.Subscriptions()))
	}

	sub1 := yt.Subscription{
		ChannelID: "UC111",
		Title:     "Channel One",
		URL:       "https://youtube.com/channel/UC111",
	}

	if err := s.AddSubscription(sub1); err != nil {
		t.Fatalf("AddSubscription failed: %v", err)
	}

	if !s.HasSubscription("UC111") {
		t.Errorf("expected HasSubscription(UC111) to be true")
	}
	if s.HasSubscription("UC222") {
		t.Errorf("expected HasSubscription(UC222) to be false")
	}

	// Add batch with a duplicate.
	sub2 := yt.Subscription{ChannelID: "UC222", Title: "Channel Two"}
	sub1Updated := yt.Subscription{ChannelID: "UC111", Title: "Channel One Updated"}
	if err := s.AddSubscriptions([]yt.Subscription{sub2, sub1Updated}); err != nil {
		t.Fatalf("AddSubscriptions failed: %v", err)
	}

	subs := s.Subscriptions()
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}
	if subs[0].Title != "Channel One Updated" {
		t.Errorf("expected updated title %q, got %q", "Channel One Updated", subs[0].Title)
	}

	// Remove subscription.
	if err := s.RemoveSubscription("UC111"); err != nil {
		t.Fatalf("RemoveSubscription failed: %v", err)
	}
	if s.HasSubscription("UC111") {
		t.Errorf("expected UC111 to be removed")
	}
	if len(s.Subscriptions()) != 1 {
		t.Errorf("expected 1 subscription remaining, got %d", len(s.Subscriptions()))
	}
}

func TestStoreWatchLater(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	v1 := yt.Video{ID: "vid1", Title: "Video 1"}
	v2 := yt.Video{ID: "vid2", Title: "Video 2"}

	if err := s.AddWatchLater(v1); err != nil {
		t.Fatalf("AddWatchLater failed: %v", err)
	}
	if err := s.AddWatchLater(v2); err != nil {
		t.Fatalf("AddWatchLater failed: %v", err)
	}

	wl := s.WatchLater()
	if len(wl) != 2 {
		t.Fatalf("expected 2 watch later items, got %d", len(wl))
	}
	// Newest first.
	if wl[0].ID != "vid2" || wl[1].ID != "vid1" {
		t.Errorf("expected newest first order [vid2, vid1], got [%s, %s]", wl[0].ID, wl[1].ID)
	}

	if !s.IsInWatchLater("vid1") {
		t.Errorf("expected IsInWatchLater(vid1) true")
	}

	// Toggle: should remove vid1.
	added, err := s.ToggleWatchLater(v1)
	if err != nil {
		t.Fatalf("ToggleWatchLater failed: %v", err)
	}
	if added {
		t.Errorf("expected toggle to remove vid1 (added=false), got added=true")
	}
	if s.IsInWatchLater("vid1") {
		t.Errorf("expected vid1 to be removed after toggle")
	}

	// Toggle again: should add vid1 back to front.
	added, err = s.ToggleWatchLater(v1)
	if err != nil {
		t.Fatalf("ToggleWatchLater add failed: %v", err)
	}
	if !added {
		t.Errorf("expected toggle to add vid1 back (added=true), got added=false")
	}
	if s.WatchLater()[0].ID != "vid1" {
		t.Errorf("expected vid1 to be at front after toggle add")
	}
}

func TestStoreHistoryMax500(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	// Add 550 entries.
	for i := 0; i < 550; i++ {
		v := yt.Video{
			ID:    fmt.Sprintf("vid-%03d", i),
			Title: fmt.Sprintf("Title %d", i),
		}
		if err := s.AddHistory(v); err != nil {
			t.Fatalf("AddHistory failed at %d: %v", i, err)
		}
	}

	hist := s.History()
	if len(hist) != 500 {
		t.Fatalf("expected history capped at 500 items, got %d", len(hist))
	}
	// Newest should be vid-549.
	if hist[0].ID != "vid-549" {
		t.Errorf("expected newest to be vid-549, got %s", hist[0].ID)
	}
	// Oldest should be vid-050.
	if hist[499].ID != "vid-050" {
		t.Errorf("expected oldest to be vid-050, got %s", hist[499].ID)
	}

	// Re-adding an existing video should bring it to the front without growing length.
	vReplay := yt.Video{ID: "vid-100", Title: "Title 100 Replayed"}
	if err := s.AddHistory(vReplay); err != nil {
		t.Fatalf("AddHistory replay failed: %v", err)
	}

	histAfter := s.History()
	if len(histAfter) != 500 {
		t.Errorf("expected length to remain 500 after replay, got %d", len(histAfter))
	}
	if histAfter[0].ID != "vid-100" {
		t.Errorf("expected vid-100 at front after replay, got %s", histAfter[0].ID)
	}
}

func TestStoreReloadFromDisk(t *testing.T) {
	dir := t.TempDir()

	s1, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("NewWithDir failed: %v", err)
	}

	_ = s1.AddSubscription(yt.Subscription{ChannelID: "UC999", Title: "Persisted Channel"})
	_ = s1.AddWatchLater(yt.Video{ID: "wl-persist", Title: "Persisted WL"})
	_ = s1.AddHistory(yt.Video{ID: "hist-persist", Title: "Persisted Hist"})

	// Reopen store in same directory.
	s2, err := NewWithDir(dir)
	if err != nil {
		t.Fatalf("reloading store failed: %v", err)
	}

	if !s2.HasSubscription("UC999") {
		t.Errorf("reloaded store missing subscription")
	}
	if !s2.IsInWatchLater("wl-persist") {
		t.Errorf("reloaded store missing watch later item")
	}
	if len(s2.History()) != 1 || s2.History()[0].ID != "hist-persist" {
		t.Errorf("reloaded store missing history item")
	}

	// Verify no temporary files left in dir.
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp.") {
			t.Errorf("found leftover tmp file: %s", entry.Name())
		}
	}
}
