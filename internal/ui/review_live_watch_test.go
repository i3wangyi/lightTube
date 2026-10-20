package ui

import (
	"errors"
	"image"
	"net/http"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"lighttube/internal/store"
	"lighttube/internal/thumbs"
	"lighttube/internal/yt"
)

func TestReviewLiveWatchHeroExcludesOtherSpaces(t *testing.T) {
	ui := reviewUI(t)
	reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
	ui.state.SetVideos([]yt.Video{{ID: "watch_feed", Title: "Watch feed title", Channel: "Watch creator"}})
	if err := ui.store.SaveMediaPosition(ui.currentAccountID(), "music_position", store.Position{Seconds: 42, Duration: 600, Space: "Music", MediaKind: "track", Title: "Music title", Show: "Music creator"}); err != nil {
		t.Fatal(err)
	}
	hero, _, ok := ui.getWatchHero()
	if !ok || hero.ID != "watch_feed" {
		t.Fatalf("Watch hero=%+v, want Watch feed instead of Music position", hero)
	}
}

func TestReviewLiveWatchHeroUsesStoredMetadata(t *testing.T) {
	ui := reviewUI(t)
	reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
	if err := ui.store.SaveMediaPosition(ui.currentAccountID(), "watch_position", store.Position{Seconds: 42, Duration: 600, Space: "Watch", MediaKind: "video", Title: "Watch title", Show: "Watch creator"}); err != nil {
		t.Fatal(err)
	}
	hero, _, ok := ui.getWatchHero()
	if !ok || hero.Title != "Watch title" || hero.Channel != "Watch creator" {
		t.Fatalf("Watch hero lost stored metadata: %+v", hero)
	}
}

func TestReviewLiveWatchHeroHasNoDemoSubtitle(t *testing.T) {
	ui := reviewUI(t)
	reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
	if err := ui.store.SaveMediaPosition(ui.currentAccountID(), "watch_position", store.Position{Seconds: 42, Duration: 600, Space: "Watch", MediaKind: "video", Title: "Watch title", Show: "Watch creator"}); err != nil {
		t.Fatal(err)
	}
	hero, pos, _ := ui.getWatchHero()
	var router input.Router
	var ops op.Ops
	ui.layoutHeroCopy(layout.Context{Ops: &ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(600, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now()}, hero, pos)
	router.Frame(&ops)
	labels := []string{}
	var scan func([]input.SemanticNode)
	scan = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if n.Desc.Label != "" {
				labels = append(labels, n.Desc.Label)
			}
			scan(n.Children)
		}
	}
	scan(router.AppendSemantics(nil))
	if len(labels) == 0 {
		t.Fatal("fixture captured no rendered text")
	}
	for _, label := range labels {
		if strings.Contains(label, "Architecture & city life") {
			t.Fatalf("demo subtitle in live Watch semantic text: %q", label)
		}
	}
}

type reviewOfflineThumbTransport struct{}

func (reviewOfflineThumbTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline thumbnail fixture")
}

func TestReviewLiveWatchRequestsVisibleThumbnails(t *testing.T) {
	for _, part := range []string{"hero", "grid"} {
		t.Run(part, func(t *testing.T) {
			ui := reviewUI(t)
			reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
			manager := thumbs.NewWithOptions(&http.Client{Transport: reviewOfflineThumbTransport{}}, t.TempDir(), thumbs.DefaultLRUByteBudget)
			ui.thumbs = manager
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(1200, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now()}
			video := yt.Video{ID: "visible_item", Title: "Visible item", Channel: "Fixture creator"}
			if part == "hero" {
				ui.layoutHeroPlayerPreview(gtx, video, store.Position{})
			} else {
				ui.layoutResponsiveGrid(gtx, []yt.Video{video})
			}
			if got := manager.LoadCount(); got == 0 {
				t.Fatalf("visible live Watch %s made %d thumbnail load requests", part, got)
			}
		})
	}
}
