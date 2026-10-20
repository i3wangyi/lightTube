package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lighttube/internal/yt"
)

type reviewDelayedYTDLP struct {
	started chan string
	replies map[string]chan []byte
}

func (f *reviewDelayedYTDLP) Search(ctx context.Context, query string) ([]byte, error) {
	f.started <- query
	select {
	case result := <-f.replies[query]:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (f *reviewDelayedYTDLP) ResolveChannel(context.Context, string) (string, error) { return "", nil }
func reviewWaitFor(t *testing.T, ui *UI, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		ui.drainActions()
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for fixture response")
		case <-tick.C:
		}
	}
}

// A stale response can be discarded before it queues an action. Wait for a
// bounded settling interval instead of requiring the known-bad state to appear.
func reviewDrainResponses(ui *UI) {
	deadline := time.NewTimer(200 * time.Millisecond)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		ui.drainActions()
		select {
		case <-deadline.C:
			ui.drainActions()
			return
		case <-tick.C:
		}
	}
}
func reviewResult(id string) []byte {
	return []byte(fmt.Sprintf(`{"entries":[{"id":%q,"title":%q}]}`, id, id))
}
func reviewWaitStarted(t *testing.T, f *reviewDelayedYTDLP, query string) {
	t.Helper()
	select {
	case got := <-f.started:
		if got != query {
			t.Fatalf("started %q, want %q", got, query)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fixture search did not start")
	}
}
func TestFollowupLatestSearchWins(t *testing.T) {
	ui := reviewUI(t)
	f := &reviewDelayedYTDLP{started: make(chan string, 2), replies: map[string]chan []byte{"old": make(chan []byte, 1), "new": make(chan []byte, 1)}}
	ui.ytClient = yt.NewClientWithOptions(f, nil, t.TempDir())
	ui.search("old")
	reviewWaitStarted(t, f, "old")
	ui.search("new")
	reviewWaitStarted(t, f, "new")
	f.replies["new"] <- reviewResult("new_result1")
	reviewWaitFor(t, ui, func() bool { return len(ui.state.Videos) == 1 && ui.state.Videos[0].ID == "new_result1" })
	f.replies["old"] <- reviewResult("old_result1")
	reviewDrainResponses(ui)
	if ui.state.Videos[0].ID != "new_result1" {
		t.Fatalf("older response replaces newer results: %s", ui.state.Videos[0].ID)
	}
}
func TestFollowupWatchResponseRetainedWhileAway(t *testing.T) {
	ui := reviewUI(t)
	f := &reviewDelayedYTDLP{started: make(chan string, 1), replies: map[string]chan []byte{"late": make(chan []byte, 1)}}
	ui.ytClient = yt.NewClientWithOptions(f, nil, t.TempDir())
	watch := ui.spaces.State(SpaceWatch)
	watch.ContentLoaded = true
	watch.Videos = []yt.Video{{ID: "baseline001"}}
	ui.spaces.State(SpaceMusic).ContentLoaded = true
	ui.searchEditor.SetText("late")
	ui.search("late")
	reviewWaitStarted(t, f, "late")
	ui.switchSpace(SpaceMusic)
	f.replies["late"] <- reviewResult("late_result")
	reviewDrainResponses(ui)
	ui.switchSpace(SpaceWatch)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != "late_result" {
		t.Fatalf("returned query=%q with stale results=%+v", ui.searchEditor.Text(), ui.state.Videos)
	}
}
func TestFollowupPodcastContinueExcludesOtherSpaces(t *testing.T) {
	ui := reviewUI(t)
	p := &sequencePlayer{}
	ui.embedPlayer = p
	for _, item := range []struct {
		space SpaceID
		id    string
	}{{SpaceWatch, "watch_item1"}, {SpaceMusic, "music_item1"}, {SpacePodcasts, "podcast_ep1"}} {
		ui.spaces.ActiveSpace = item.space
		ui.playSpaceVideo(yt.Video{ID: item.id, Title: item.id, Duration: 600})
		p.position = 120
		ui.saveCurrentPosition()
	}
	ui.loadPodcastsSpaceContent(2)
	episodes := ui.spaces.State(SpacePodcasts).Episodes
	if len(episodes) != 1 || episodes[0].ID != "podcast_ep1" {
		t.Fatalf("Podcasts Continue contains non-podcast media: %+v", episodes)
	}
}
func TestFollowupWatchSavedSurvivesSpaceReturn(t *testing.T) {
	ui := reviewUI(t)
	item := yt.Video{ID: "saved_item1", Title: "Saved"}
	if err := ui.store.AddWatchLater(item); err != nil {
		t.Fatal(err)
	}
	ui.spaces.State(SpaceWatch).Chip = 3
	ui.loadWatchSpaceContent(3)
	if len(ui.state.Videos) != 1 {
		t.Fatal("saved fixture did not load")
	}
	ui.spaces.State(SpaceMusic).ContentLoaded = true
	ui.switchSpace(SpaceMusic)
	ui.switchSpace(SpaceWatch)
	if len(ui.state.Videos) != 1 || ui.state.Videos[0].ID != item.ID {
		t.Fatalf("Watch Saved lost on return: %+v", ui.state.Videos)
	}
}
func TestFollowupSignedOutPodcastPlaylistDiscovery(t *testing.T) {
	ui := reviewUI(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "playlist-source")
	script := "#!/bin/sh\nprintf '%s' '{\"entries\":[{\"_type\":\"playlist\",\"id\":\"PLfixture\",\"title\":\"Science podcast\",\"url\":\"https://www.youtube.com/playlist?list=PLfixture\"}]}'\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ui.account = yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: filepath.Join(dir, "missing"), CacheDir: dir, BinaryPath: bin})
	playlists, err := ui.account.SearchPodcastPlaylists(context.Background(), "science")
	if err != nil || len(playlists) != 1 {
		t.Fatalf("playlist fixture unavailable: %v %+v", err, playlists)
	}
	ui.spaces.ActiveSpace = SpacePodcasts
	ui.searchPodcasts("science")
	reviewWaitFor(t, ui, func() bool { return ui.state.Status != "Searching Podcasts for \"science\"…" })
	if len(ui.spaces.State(SpacePodcasts).PodcastPlaylists) != 1 {
		t.Fatalf("signed-out playlist discovery blocked: %s", ui.state.Status)
	}
}
