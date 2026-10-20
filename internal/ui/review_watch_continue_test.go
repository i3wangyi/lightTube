package ui

import (
	"testing"

	"lighttube/internal/store"
)

func TestReviewWatchContinueDisplaysStoredTitle(t *testing.T) {
	ui := reviewUI(t)
	reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
	if err := ui.store.SaveMediaPosition(ui.currentAccountID(), "watch_item1", store.Position{Seconds: 42, Duration: 600, Space: "Watch", MediaKind: "video", Title: "A useful video title", Show: "Fixture creator"}); err != nil {
		t.Fatal(err)
	}
	ui.loadWatchSpaceContent(2)
	if len(ui.state.Videos) != 1 {
		t.Fatalf("expected one Continue item: %+v", ui.state.Videos)
	}
	if got := ui.state.Videos[0].Title; got != "A useful video title" {
		t.Fatalf("Watch Continue title=%q, want stored title %q", got, "A useful video title")
	}
}
