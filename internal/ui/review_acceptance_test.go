package ui

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"lighttube/internal/store"
	"lighttube/internal/yt"
)

type reviewPlayer struct {
	fakeEmbedRecorder
	muted bool
	seeks []float64
}

func (p *reviewPlayer) ToggleMute()                     { p.muted = !p.muted }
func (p *reviewPlayer) Seek(sec float64, relative bool) { p.seeks = append(p.seeks, sec) }

func reviewUI(t *testing.T) *UI {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewWithDir(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	// Construct the offline demo path, then supply only controlled live fixtures.
	ui := New(Options{Demo: true, Store: st})
	ui.demo = false
	ui.demoData = nil
	ui.account = nil
	ui.thumbs = nil
	ui.state.SetVideos(nil)
	return ui
}
func reviewFrame(ui *UI) {
	var ops op.Ops
	ui.layout(layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 760)), Now: time.Now()})
}

func TestReviewAccountBoundaryClearsSpaceState(t *testing.T) {
	for _, transition := range []string{"signout", "switch"} {
		t.Run(transition, func(t *testing.T) {
			ui := reviewUI(t)
			dir := t.TempDir()
			ui.account = yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: filepath.Join(dir, "missing"), CacheDir: dir})
			ui.state.CurrentTab = TabAccount
			p := &reviewPlayer{}
			ui.embedPlayer = p
			ui.watchPaused = false
			for _, sp := range ui.spaces.Spaces {
				sp.Query = "private query"
				sp.Queue = []yt.Video{{ID: "private"}}
				sp.ResumeItem = &yt.Video{ID: "private"}
				sp.Shorts = []yt.Video{{ID: "private"}}
				sp.Tracks = []yt.Video{{ID: "private"}}
				sp.Episodes = []yt.Video{{ID: "private"}}
			}
			if transition == "signout" {
				ui.signOutAccount()
			} else {
				acc := store.AccountConfig{ID: "review-second", Label: "Second", CookieFile: filepath.Join(dir, "second-missing"), CacheDir: filepath.Join(dir, "second-cache")}
				if err := ui.store.AddAccount(acc); err != nil {
					t.Fatal(err)
				}
				ui.switchAccount(acc)
			}
			for id, sp := range ui.spaces.Spaces {
				if sp.Query != "" || len(sp.Queue) > 0 || sp.ResumeItem != nil || len(sp.Shorts) > 0 || len(sp.Tracks) > 0 || len(sp.Episodes) > 0 {
					t.Errorf("%s retains private state in %s: query=%q queue=%d resume=%v", transition, id, sp.Query, len(sp.Queue), sp.ResumeItem)
				}
			}
			if !p.paused && !p.stopped {
				t.Error("account boundary leaves old player running")
			}
		})
	}
}

func TestReviewPlayerOwnershipOnReturn(t *testing.T) {
	ui := reviewUI(t)
	p := &reviewPlayer{}
	ui.embedPlayer = p
	music := yt.Video{ID: "music_test1", Title: "Music item", Duration: 240}
	podcast := yt.Video{ID: "podcast0011", Title: "Podcast item", Duration: 1800}
	ui.spaces.ActiveSpace = SpaceMusic
	ui.spaces.State(SpaceMusic).Tracks = []yt.Video{music}
	ui.selectMusicTrack(music, []yt.Video{music})
	ui.switchSpace(SpacePodcasts)
	ui.spaces.State(SpacePodcasts).Shows = []yt.PodcastShow{{ID: "show", Title: "Show"}}
	ui.spaces.State(SpacePodcasts).Episodes = []yt.Video{podcast}
	ui.selectPodcastEpisode(podcast, []yt.Video{podcast})
	ui.switchSpace(SpaceMusic)
	reviewFrame(ui)
	if len(p.plays) == 0 || len(p.hidden) == 0 {
		t.Fatal("missing player evidence")
	}
	last := p.plays[len(p.plays)-1].URL
	if !p.hidden[len(p.hidden)-1] && last != music.WatchURL() {
		t.Fatalf("Music dock exposes %s while its resume item is %s", last, music.WatchURL())
	}
}

func TestReviewEveryShortStartsMuted(t *testing.T) {
	ui := reviewUI(t)
	p := &reviewPlayer{}
	ui.embedPlayer = p
	ui.spaces.ActiveSpace = SpaceShorts
	items := []yt.Video{{ID: "short_one01", Title: "One"}, {ID: "short_two02", Title: "Two"}}
	for _, item := range items {
		ui.selectShort(item, items)
		if !p.muted {
			t.Errorf("Short %s starts with sound on", item.ID)
		}
	}
}

func TestReviewSavedShortRetainsSourceIdentity(t *testing.T) {
	ui := reviewUI(t)
	items, err := yt.ParseShortsTabJSON([]byte(`{"entries":[{"id":"short_test1","title":"Short from tab","url":"https://www.youtube.com/watch?v=short_test1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = ui.store.AddWatchLater(items[0]); err != nil {
		t.Fatal(err)
	}
	ui.loadShortsSpaceContent(1)
	if len(ui.spaces.State(SpaceShorts).Shorts) != 1 {
		t.Fatal("canonical watch URL from Shorts tab disappears from Saved Shorts")
	}
}

func TestReviewWatchSavedSignedOut(t *testing.T) {
	ui := reviewUI(t)
	item := yt.Video{ID: "saved_test1", Title: "Saved locally"}
	if err := ui.store.AddWatchLater(item); err != nil {
		t.Fatal(err)
	}
	ui.loadWatchSpaceContent(3)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != item.ID {
		t.Fatalf("Watch Saved omits local saved item: %v", ui.state.Videos)
	}
}

func TestReviewSearchResultsRestore(t *testing.T) {
	ui := reviewUI(t)
	ui.demo = true
	ui.demoData = LoadDemoData()
	ui.searchEditor.SetText("camera")
	ui.search("camera")
	before := len(ui.state.Videos)
	if before != 1 {
		t.Fatalf("bad setup: %d matches", before)
	}
	ui.switchSpace(SpaceMusic)
	ui.switchSpace(SpaceWatch)
	if ui.searchEditor.Text() != "camera" || len(ui.state.Videos) != before {
		t.Fatalf("query=%q restored with %d feed items; wanted %d search match", ui.searchEditor.Text(), len(ui.state.Videos), before)
	}
}

func TestReviewShortNavigationDoesNotKeepOldMediaRunning(t *testing.T) {
	ui := reviewUI(t)
	p := &reviewPlayer{}
	ui.embedPlayer = p
	ui.spaces.ActiveSpace = SpaceShorts
	state := ui.spaces.State(SpaceShorts)
	items := []yt.Video{{ID: "short_one01", Title: "One"}, {ID: "short_two02", Title: "Two"}}
	state.Shorts = items
	ui.selectShort(items[0], items)
	ui.spaces.ShortsNextClick.Click()
	reviewFrame(ui)
	reviewFrame(ui)
	if state.ShortIndex != 1 {
		t.Fatal("Next did not change selection")
	}
	last := p.plays[len(p.plays)-1].URL
	if !p.paused && last != items[1].WatchURL() {
		t.Fatalf("Next selects %s while unpaused player still runs %s", items[1].ID, last)
	}
}

func TestReviewSavedShortPlayerVisible(t *testing.T) {
	ui := reviewUI(t)
	p := &reviewPlayer{}
	ui.embedPlayer = p
	ui.spaces.ActiveSpace = SpaceShorts
	state := ui.spaces.State(SpaceShorts)
	state.Chip = 1
	item := yt.Video{ID: "saved_short", Title: "Saved Short", URL: "https://www.youtube.com/shorts/saved_short"}
	state.Shorts = []yt.Video{item}
	ui.selectShort(item, state.Shorts)
	reviewFrame(ui)
	if len(p.hidden) == 0 || p.hidden[len(p.hidden)-1] {
		t.Fatal("Saved Short playback is active but its video is hidden")
	}
}

func TestReviewResumeUsesStoredPosition(t *testing.T) {
	ui := reviewUI(t)
	p := &reviewPlayer{}
	ui.embedPlayer = p
	ui.spaces.ActiveSpace = SpaceMusic
	item := yt.Video{ID: "resume_test", Title: "Resume this", Duration: 240}
	if err := ui.store.SavePosition("", item.ID, 42, 240); err != nil {
		t.Fatal(err)
	}
	ui.selectMusicTrack(item, []yt.Video{item})
	for _, s := range p.seeks {
		if s == 42 {
			return
		}
	}
	t.Fatalf("stored position 42 never applied; seeks=%v time=%.1f", p.seeks, ui.watchCurrentTime)
}
