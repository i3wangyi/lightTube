package ui

import (
	"testing"

	"lighttube/internal/store"
)

// Watch Continue lists Watch items and legacy records, never items that
// another space owns, and falls back to the video ID without a title.
func TestWatchContinueScopesToWatchAndLegacy(t *testing.T) {
	ui := reviewUI(t)
	reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
	acc := ui.currentAccountID()
	saves := map[string]store.Position{
		"watch_item1":   {Seconds: 42, Duration: 600, Space: "Watch", MediaKind: "video", Title: "Watch title"},
		"podcast_item1": {Seconds: 42, Duration: 600, Space: "Podcasts", MediaKind: "episode", Title: "Episode"},
		"music_item1":   {Seconds: 42, Duration: 600, Space: "Music", MediaKind: "track", Title: "Track"},
		"legacy_item1":  {Seconds: 42, Duration: 600},
	}
	for id, p := range saves {
		if err := ui.store.SaveMediaPosition(acc, id, p); err != nil {
			t.Fatal(err)
		}
	}
	ui.loadWatchSpaceContent(2)
	got := map[string]string{}
	for _, v := range ui.state.Videos {
		got[v.ID] = v.Title
	}
	want := map[string]string{"watch_item1": "Watch title", "legacy_item1": "legacy_item1"}
	if len(got) != len(want) {
		t.Fatalf("Watch Continue = %v, want %v", got, want)
	}
	for id, title := range want {
		if got[id] != title {
			t.Fatalf("Watch Continue = %v, want %v", got, want)
		}
	}
}
