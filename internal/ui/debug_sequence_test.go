package ui

import (
	"reflect"
	"testing"
	"time"

	"gioui.org/widget"
	"lighttube/internal/embed"
	"lighttube/internal/yt"
)

type sequencePlayer struct {
	reviewPlayer
	position float64
}

func (p *sequencePlayer) Play(opts embed.PlayOptions) error {
	p.position, p.paused = 0, false
	return p.fakeEmbedRecorder.Play(opts)
}
func (p *sequencePlayer) Seek(seconds float64, relative bool) {
	p.position = seconds
	p.reviewPlayer.Seek(seconds, relative)
}
func (p *sequencePlayer) Status() embed.Status {
	return embed.Status{TimePos: p.position, Duration: 600, Paused: p.paused, Muted: p.muted, IsPlaying: len(p.plays) > 0}
}

func TestDebugSequenceMatchesClickHandlers(t *testing.T) {
	setup := func() (*UI, *sequencePlayer) {
		ui := reviewUI(t)
		p := &sequencePlayer{}
		ui.embedPlayer = p
		ui.firstFramePresented = true
		for space, items := range map[SpaceID][]yt.Video{
			SpaceMusic:    {{ID: "music_first", Title: "First track", Duration: 600}, {ID: "music_second"}},
			SpacePodcasts: {{ID: "podcast_first", Title: "First episode", Duration: 600}, {ID: "podcast_second"}},
		} {
			st := ui.spaces.State(space)
			st.ContentLoaded = true
			st.Tracks, st.Episodes = items, items
		}
		return ui, p
	}
	hook, hp := setup()
	click, cp := setup()
	hook.debugSequence = newDebugSequence("music:play,wait:8,podcasts:play,wait:8,music:show,wait:4,music:play,wait:8")
	hook.debugSequence.ready = true
	now := time.Now()
	clickFrame := func(btn *widget.Clickable) {
		btn.Click()
		reviewFrame(click)
		reviewFrame(click) // Row selection is queued during layout.
	}
	for _, step := range hook.debugSequence.steps {
		switch step {
		case "music:play":
			if click.spaces.ActiveSpace != SpaceMusic {
				clickFrame(&click.spaceNavClicks[SpaceMusic])
			}
			if click.spaces.Active().ResumeItem == nil {
				clickFrame(&click.spaces.TrackClicks[0])
			} else {
				clickFrame(&click.spaces.MusicDockPlay)
			}
		case "podcasts:play":
			clickFrame(&click.spaceNavClicks[SpacePodcasts])
			clickFrame(&click.spaces.EpisodeClicks[0])
		case "music:show":
			clickFrame(&click.spaceNavClicks[SpaceMusic])
		default:
			// A timer cannot advance the sequence before its deadline.
			if got := hook.advanceDebugSequence(now); got != "" {
				t.Fatalf("wait completed early: %s", got)
			}
			now = now.Add(8 * time.Second)
		}
		if got := hook.advanceDebugSequence(now); got != step {
			t.Fatalf("step=%q, want %q", got, step)
		}
		reviewFrame(hook)
		if hook.spaces.ActiveSpace != click.spaces.ActiveSpace || hook.playerOwnerSpace != click.playerOwnerSpace || hook.playerOwnerID != click.playerOwnerID || hook.activeVideoBox != click.activeVideoBox || hp.paused != cp.paused || !reflect.DeepEqual(hp.plays, cp.plays) || !reflect.DeepEqual(hp.seeks, cp.seeks) {
			t.Fatalf("hook differs from clicks after %s: hook owner=%s/%s rect=%v paused=%t; click owner=%s/%s rect=%v paused=%t", step, hook.playerOwnerSpace, hook.playerOwnerID, hook.activeVideoBox, hp.paused, click.playerOwnerSpace, click.playerOwnerID, click.activeVideoBox, cp.paused)
		}
		if step == "music:show" && (!hp.paused || !hook.activeVideoBox.Empty()) {
			t.Fatal("return to Music exposes the Podcast player or leaves it playing")
		}
		if step == "music:play" && len(hp.plays) == 3 && (hp.paused || hp.position != 18 || hook.activeVideoBox.Empty()) {
			t.Fatalf("dock resume paused or lost its position: paused=%t position=%v rect=%v", hp.paused, hp.position, hook.activeVideoBox)
		}
		if step == "wait:8" {
			hp.position, cp.position = 18, 18
			hook.watchCurrentTime, click.watchCurrentTime = 18, 18
		}
		now = now.Add(time.Second)
	}
	if len(hp.plays) != 3 || hp.plays[2].URL != "https://www.youtube.com/watch?v=music_first" || hp.position != 18 {
		t.Fatalf("Music did not resume its saved position: plays=%+v position=%v", hp.plays, hp.position)
	}
}

func TestDebugSequenceDisabledAndMissingItems(t *testing.T) {
	ui := reviewUI(t)
	if ui.debugSequence != nil || ui.advanceDebugSequence(time.Now()) != "" {
		t.Fatal("sequence enabled without flag")
	}
	ui.firstFramePresented = true
	ui.debugSequence = newDebugSequence("music:play")
	ui.debugSequence.ready = true
	now := time.Now()
	if got := ui.advanceDebugSequence(now); got != "" {
		t.Fatalf("played without a real item: %s", got)
	}
	if got := ui.advanceDebugSequence(now.Add(46 * time.Second)); got != "music:play ERROR=no real item loaded within 45 seconds" {
		t.Fatalf("missing item did not fail: %s", got)
	}
}

func TestDockClickResumesBeforeVideoVisibility(t *testing.T) {
	for _, space := range []SpaceID{SpaceMusic, SpacePodcasts} {
		t.Run(space.String(), func(t *testing.T) {
			ui := reviewUI(t)
			p := &sequencePlayer{}
			ui.embedPlayer = p
			item := yt.Video{ID: "target_item", Title: "Target", Duration: 600}
			other := yt.Video{ID: "other_item", Title: "Other", Duration: 600}
			target := ui.spaces.State(space)
			target.ContentLoaded = true
			ui.spaces.ActiveSpace = space
			ui.playSpaceVideo(item)
			p.position = 37
			away := SpaceMusic
			if space == SpaceMusic {
				away = SpacePodcasts
			}
			ui.spaces.State(away).ContentLoaded = true
			ui.switchSpace(away)
			ui.playSpaceVideo(other)
			ui.switchSpace(space)
			reviewFrame(ui)
			if !ui.activeVideoBox.Empty() || !p.paused {
				t.Fatal("return exposes the other space's player")
			}
			if space == SpaceMusic {
				ui.spaces.MusicDockPlay.Click()
			} else {
				ui.spaces.PodcastsDockPlay.Click()
			}
			reviewFrame(ui)
			if p.paused || ui.activeVideoBox.Empty() || p.position != 37 || ui.playerOwnerID != item.ID {
				t.Fatalf("dock click did not resume visibly in one frame: paused=%t rect=%v position=%v owner=%s", p.paused, ui.activeVideoBox, p.position, ui.playerOwnerID)
			}
		})
	}
}
