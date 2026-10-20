package ui

import (
	"testing"

	"lighttube/internal/yt"
)

func TestCorrectionMusicChipsUseSeparateSources(t *testing.T) {
	ui := reviewUI(t)
	album := yt.Album{ID: "album-fixture", Title: "Fixture album", Artist: "Fixture artist", Tracks: []yt.Video{{ID: "track_fixture", Title: "Fixture track"}}}
	if err := ui.store.SaveCollection("", album); err != nil {
		t.Fatal(err)
	}

	ui.loadMusicSpaceContent(0) // Liked Music. No signed-in source is present.
	state := ui.spaces.State(SpaceMusic)
	if len(state.Tracks) != 0 {
		t.Fatalf("Liked chip used another source: %+v", state.Tracks)
	}
	ui.loadMusicSpaceContent(1) // Last Music search.
	if len(state.Tracks) != 0 {
		t.Fatalf("Albums chip used another source: %+v", state.Tracks)
	}
	ui.loadMusicSpaceContent(2) // Saved collections.
	if state.Album == nil || state.Album.ID != album.ID || len(state.Tracks) != 1 || state.Tracks[0].ID != "track_fixture" {
		t.Fatalf("Saved chip missed its stored collection: album=%+v tracks=%+v", state.Album, state.Tracks)
	}
}

func TestCorrectionPodcastChipsUseSeparateSources(t *testing.T) {
	ui := reviewUI(t)
	if err := ui.store.SavePosition("", "episode_continue", 120, 600); err != nil {
		t.Fatal(err)
	}
	if err := ui.store.SavePosition("", "episode_played", 590, 600); err != nil {
		t.Fatal(err)
	}

	state := ui.spaces.State(SpacePodcasts)
	ui.loadPodcastsSpaceContent(0) // Your shows.
	if len(state.Shows) != 0 || len(state.Episodes) != 0 {
		t.Fatalf("Your shows used an unavailable source: %+v", state)
	}
	ui.loadPodcastsSpaceContent(1) // New episodes.
	if len(state.Shows) != 0 || len(state.Episodes) != 0 {
		t.Fatalf("New episodes used another source: %+v", state)
	}
	ui.loadPodcastsSpaceContent(2) // Continue.
	if len(state.Episodes) != 1 || state.Episodes[0].ID != "episode_continue" {
		t.Fatalf("Continue did not use unfinished positions: %+v", state.Episodes)
	}
	if len(state.Queue) != 1 || state.Queue[0].ID != state.Episodes[0].ID {
		t.Fatalf("Continue queue differs from results: %+v", state.Queue)
	}
}

func TestCorrectionNarrowTimeAndShortsScrimStyles(t *testing.T) {
	ui := reviewUI(t)
	if got := ui.oneLineLabel(12, "99 min left").MaxLines; got != 1 {
		t.Fatalf("time label max lines=%d, want 1", got)
	}
	scrim := shortsStageScrim(565)
	if scrim.Stop2.Y != 565 || scrim.Color1.A == scrim.Color2.A {
		t.Fatalf("Shorts scrim is not a varying smooth gradient: %+v", scrim)
	}
}

func TestCorrectionDockPlayReloadsItsItemAndResumePosition(t *testing.T) {
	ui := reviewUI(t)
	p := &reviewPlayer{}
	ui.embedPlayer = p
	music := yt.Video{ID: "music_resume1", Title: "Music", Duration: 240}
	podcast := yt.Video{ID: "podcast_resume", Title: "Podcast", Duration: 1800}
	if err := ui.store.SavePosition("", music.ID, 42, 240); err != nil {
		t.Fatal(err)
	}
	ui.spaces.ActiveSpace = SpaceMusic
	ui.selectMusicTrack(music, []yt.Video{music})
	ui.switchSpace(SpacePodcasts)
	ui.selectPodcastEpisode(podcast, []yt.Video{podcast})
	ui.switchSpace(SpaceMusic)
	plays := len(p.plays)
	if p.plays[plays-1].URL != podcast.WatchURL() {
		t.Fatalf("return to Music changed playback: %s", p.plays[plays-1].URL)
	}
	ui.spaces.MusicDockPlay.Click()
	reviewFrame(ui)
	if len(p.plays) != plays+1 || p.plays[len(p.plays)-1].URL != music.WatchURL() {
		t.Fatalf("Music dock Play did not load its item: %+v", p.plays)
	}
	if got, ok := ui.store.Position("", music.ID); !ok || got.Seconds != 42 {
		t.Fatalf("Music position changed while another item loaded: %+v", got)
	}
	for _, seek := range p.seeks {
		if seek == 42 {
			return
		}
	}
	t.Fatalf("Music item did not seek to saved position: %v", p.seeks)
}
