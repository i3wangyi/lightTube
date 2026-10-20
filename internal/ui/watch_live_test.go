package ui

import (
	"fmt"
	"image"
	"slices"
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

type watchCountingThumbs struct {
	*thumbs.Manager
	requests  map[string]int
	callbacks map[string]func(image.Image, error)
	images    map[string]image.Image
}

func (m *watchCountingThumbs) Get(id string) (image.Image, bool) {
	img, ok := m.images[id]
	return img, ok
}
func (m *watchCountingThumbs) Load(id string, done func(image.Image, error)) {
	m.requests[id]++
	m.callbacks[id] = done
}

func TestLiveWatchRequestsOnlyVisibleThumbnails(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		width, height, first int
		want                 []int
	}{
		{"hero and three columns", 1200, 760, 0, []int{0, 1, 2, 3, 4, 5, 6}},
		{"scrolled grid", 1200, 760, 3, []int{4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{"hero only", 500, 300, 0, []int{0}},
		{"budget", 1200, 20000, 0, func() []int {
			indices := make([]int, thumbs.MaxThumbnailsInHalfBudget)
			for i := range indices {
				indices[i] = i
			}
			return indices
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui := reviewUI(t)
			reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
			manager := &watchCountingThumbs{requests: map[string]int{}, callbacks: map[string]func(image.Image, error){}, images: map[string]image.Image{}}
			ui.thumbs = manager
			videos := make([]yt.Video, 60)
			for i := range videos {
				videos[i] = yt.Video{ID: fmt.Sprintf("video_%02d", i), Title: fmt.Sprintf("Live title %02d", i), Channel: "Live creator", Duration: 600}
			}
			ui.setWatchVideos(videos)
			ui.spaces.Active().ScrollList.Position.First = tc.first
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				ui.layoutWatchSpace(layout.Context{Ops: &ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(tc.width, tc.height)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now()})
				router.Frame(&ops)
			}
			frame()
			want := map[string]bool{}
			for _, i := range tc.want {
				want[videos[i].ID] = true
			}
			if len(manager.requests) != len(want) {
				t.Fatalf("requests=%v, want=%v; list=%+v", manager.requests, want, ui.spaces.Active().ScrollList.Position)
			}
			for id, count := range manager.requests {
				if !want[id] || count != 1 {
					t.Errorf("request %s count=%d, want only visible %v", id, count, want)
				}
				if manager.callbacks[id] == nil {
					t.Fatalf("%s has no completion callback", id)
				}
			}
			frame()
			for id, count := range manager.requests {
				if count != 1 {
					t.Errorf("in-flight %s loaded %d times", id, count)
				}
			}
			for id, done := range manager.callbacks {
				img := image.NewRGBA(image.Rect(0, 0, 32, 18))
				manager.images[id] = img
				done(img, nil)
			}
			frame()
			for id := range want {
				if _, ok := ui.imageOps[id]; !ok {
					t.Errorf("loaded visible thumbnail %s was not drawn", id)
				}
			}
			if len(manager.requests) > thumbs.MaxThumbnailsInHalfBudget {
				t.Fatal("thumbnail budget exceeded")
			}
		})
	}
}

func TestWatchHeroMergesFeedAndStoredMetadata(t *testing.T) {
	for _, space := range []string{"Watch", ""} {
		t.Run(space, func(t *testing.T) {
			ui := reviewUI(t)
			if err := ui.store.SaveMediaPosition(ui.currentAccountID(), "watch", store.Position{Seconds: 42, Duration: 600, Space: space, MediaKind: "video", Title: "Stored title", Show: "Stored creator"}); err != nil {
				t.Fatal(err)
			}
			for _, other := range []string{"Music", "Podcasts", "Shorts"} {
				if err := ui.store.SaveMediaPosition(ui.currentAccountID(), other, store.Position{Seconds: 42, Duration: 600, Space: other, Title: other + " title"}); err != nil {
					t.Fatal(err)
				}
			}
			ui.state.SetVideos([]yt.Video{{ID: "watch", Title: "watch", ChannelID: "creator_id"}})
			hero, pos, ok := ui.getWatchHero()
			if !ok || hero.ID != "watch" || hero.Title != "Stored title" || hero.Channel != "Stored creator" || hero.Duration != 600 || hero.ChannelID != "creator_id" || pos.Seconds != 42 {
				t.Fatalf("hero=%+v position=%+v", hero, pos)
			}
			ui.state.SetVideos([]yt.Video{{ID: "watch", Title: "Feed title", Channel: "Feed creator", Duration: 900}})
			hero, _, _ = ui.getWatchHero()
			if hero.Title != "Feed title" || hero.Channel != "Feed creator" || hero.Duration != 900 {
				t.Fatalf("feed metadata lost: %+v", hero)
			}
		})
	}
}

func TestWatchFeedHeroDoesNotResumeAnotherSpace(t *testing.T) {
	ui := reviewUI(t)
	if err := ui.store.SaveMediaPosition(ui.currentAccountID(), "shared", store.Position{Seconds: 42, Duration: 600, Space: "Music", Title: "Music title"}); err != nil {
		t.Fatal(err)
	}
	ui.state.SetVideos([]yt.Video{{ID: "shared", Title: "Watch feed title"}})
	hero, pos, ok := ui.getWatchHero()
	if !ok || hero.ID != "shared" || pos.Seconds != 0 {
		t.Fatalf("hero=%+v position=%+v", hero, pos)
	}
}

func TestWatchGridRetainsAllFeedItems(t *testing.T) {
	ui := reviewUI(t)
	videos := make([]yt.Video, 60)
	for i := range videos {
		videos[i] = yt.Video{ID: fmt.Sprintf("item_%02d", i), Title: fmt.Sprintf("Feed title %02d", i)}
	}
	ui.setWatchVideos(videos)
	var labels []string
	ui.labelHook = func(s string) { labels = append(labels, s) }
	var ops op.Ops
	frame := func() {
		ops.Reset()
		ui.layoutWatchSpace(layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(1200, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now()})
	}
	frame()
	if !slices.Contains(labels, "59 videos") {
		t.Fatalf("grid count absent: %q", labels)
	}
	ui.spaces.Active().ScrollList.ScrollTo(21)
	labels = nil
	frame()
	if len(ui.state.Videos) != 60 || !slices.Contains(labels, videos[59].Title) {
		t.Fatalf("last feed item missing: count=%d labels=%q", len(ui.state.Videos), labels)
	}
}
