package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

// Each source has its own gate so completion order is independent of launch order.
func delayedChipSource(t *testing.T, id string) (*yt.Account, string, func()) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("cookies", "# synthetic offline fixture\n")
	write("channels.json", `[{"id":"UCfixture","url":"https://www.youtube.com/@fixture"}]`)
	write("podcasts/feed.json", `{"entries":[{"id":"PLfixture","title":"Fixture show"}]}`)
	write("result.json", fmt.Sprintf(`{"title":"Fixture","entries":[{"id":%q,"title":%q,"duration":600}]}`, id, id))
	write("source", `#!/bin/sh
cd "$(dirname "$0")" || exit 1
: > started
while [ ! -e release ]; do sleep 0.01; done
cat result.json
`)
	release := func() { write("release", "") }
	t.Cleanup(release)
	return yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: filepath.Join(dir, "cookies"), CacheDir: dir, BinaryPath: filepath.Join(dir, "source")}), filepath.Join(dir, "started"), release
}

func waitChipStarted(t *testing.T, ui *UI, path string) {
	t.Helper()
	reviewWaitFor(t, ui, func() bool { _, err := os.Stat(path); return err == nil })
}

func waitChipCallback(t *testing.T, ui *UI) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ui.actionsMu.Lock()
		ready := len(ui.actions) > 0
		ui.actionsMu.Unlock()
		if ready {
			ui.drainActions()
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("fixture callback did not arrive")
}

func chipLoad(ui *UI, space SpaceID, chip int) {
	ui.spaces.State(space).Chip = chip
	switch space {
	case SpaceWatch:
		ui.loadWatchSpaceContent(chip)
	case SpaceShorts:
		ui.loadShortsSpaceContent(chip)
	case SpacePodcasts:
		ui.loadPodcastsSpaceContent(chip)
	case SpaceMusic:
		ui.loadMusicSpaceContent(chip)
	}
}
func chipItems(ui *UI, space SpaceID) []yt.Video {
	state := ui.spaces.State(space)
	switch space {
	case SpaceWatch:
		return state.Videos
	case SpaceShorts:
		return state.Shorts
	case SpacePodcasts:
		return state.Episodes
	default:
		return state.Tracks
	}
}

func TestChipLoadsLatestResponseWins(t *testing.T) {
	for _, space := range []SpaceID{SpaceWatch, SpaceShorts, SpacePodcasts, SpaceMusic} {
		t.Run(space.String(), func(t *testing.T) {
			ui := reviewUI(t)
			reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
			ui.spaces.ActiveSpace = space
			old, oldStarted, releaseOld := delayedChipSource(t, "old_fixture")
			newer, newStarted, releaseNew := delayedChipSource(t, "new_fixture")
			ui.account = old
			chipLoad(ui, space, 0)
			waitChipStarted(t, ui, oldStarted)
			// Replace the fixture transport, keeping the same UI account generation.
			ui.account = newer
			chipLoad(ui, space, 0)
			waitChipStarted(t, ui, newStarted)
			releaseNew()
			waitChipCallback(t, ui)
			if got := chipItems(ui, space); len(got) != 1 || got[0].ID != "new_fixture" {
				t.Fatalf("new response missing: %+v", got)
			}
			releaseOld()
			waitChipCallback(t, ui)
			if got := chipItems(ui, space); len(got) != 1 || got[0].ID != "new_fixture" {
				t.Fatalf("old response replaced newer content: %+v", got)
			}
		})
	}
}

func TestChipSwitchInvalidatesPendingFeed(t *testing.T) {
	for _, space := range []SpaceID{SpaceWatch, SpaceShorts, SpacePodcasts, SpaceMusic} {
		t.Run(space.String(), func(t *testing.T) {
			ui := reviewUI(t)
			reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
			ui.spaces.ActiveSpace = space
			source, started, release := delayedChipSource(t, "late_fixture")
			ui.account = source
			chipLoad(ui, space, 0)
			waitChipStarted(t, ui, started)
			chip := 2
			if space == SpaceShorts || space == SpaceMusic {
				chip = 1
			}
			chipLoad(ui, space, chip)
			release()
			waitChipCallback(t, ui)
			if got := chipItems(ui, space); len(got) != 0 {
				t.Fatalf("late feed replaced chip: %+v", got)
			}
		})
	}
}

func TestPodcastContinueRetainsMetadataAndExcludesLegacy(t *testing.T) {
	ui := reviewUI(t)
	reviewWaitFor(t, ui, func() bool { return ui.capsLoaded })
	if err := ui.store.SavePosition("", "legacy", 30, 600); err != nil {
		t.Fatal(err)
	}
	if err := ui.store.SaveMediaPosition("", "episode", store.Position{Seconds: 42, Duration: 600, Space: "Podcasts", MediaKind: "episode", Title: "Episode title", Show: "Show title"}); err != nil {
		t.Fatal(err)
	}
	ui.loadPodcastsSpaceContent(2)
	items := ui.spaces.State(SpacePodcasts).Episodes
	if len(items) != 1 || items[0].Title != "Episode title" || items[0].Channel != "Show title" || items[0].Duration != 600 {
		t.Fatalf("wrong Continue metadata: %+v", items)
	}
}
