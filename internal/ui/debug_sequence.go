package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"lighttube/internal/embed"
)

// debugSequence advances only from the UI event loop, including while paused.
type debugSequence struct {
	steps         []string
	index         int
	ready         bool
	done          bool
	entered       bool
	showRequested bool
	started       time.Time
}

func newDebugSequence(steps string) *debugSequence {
	if steps == "" {
		return nil
	}
	return &debugSequence{steps: strings.Split(steps, ",")}
}

func (ui *UI) advanceDebugSequence(now time.Time) string {
	seq := ui.debugSequence
	if seq == nil || !seq.ready || seq.done || !ui.firstFramePresented {
		return ""
	}
	step := strings.TrimSpace(seq.steps[seq.index])
	kind, action, ok := strings.Cut(step, ":")
	fail := func(reason string) string {
		seq.done = true
		return step + " ERROR=" + reason
	}
	if !ok {
		return fail("expected space:play, space:show, or wait:seconds")
	}
	if seq.started.IsZero() {
		seq.started = now
	}
	if kind == "wait" {
		seconds, err := strconv.ParseFloat(action, 64)
		if err != nil || !(seconds >= 0 && seconds <= 3600) {
			return fail("wait must be between 0 and 3600 seconds")
		}
		if now.Sub(seq.started).Seconds() < seconds {
			return ""
		}
	} else {
		var space SpaceID
		switch kind {
		case "watch":
			space = SpaceWatch
		case "shorts":
			space = SpaceShorts
		case "music":
			space = SpaceMusic
		case "podcasts":
			space = SpacePodcasts
		default:
			return fail("unknown space")
		}
		scrollRows := float32(0)
		if value, ok := strings.CutPrefix(action, "scroll="); ok {
			rows, err := strconv.ParseFloat(value, 32)
			if space != SpaceWatch || err != nil || !(rows >= -1000 && rows <= 1000) {
				return fail("expected watch:scroll=N, between -1000 and 1000 rows")
			}
			scrollRows, action = float32(rows), "scroll"
		}
		showIndex := -1
		if base, option, hasOption := strings.Cut(action, ":"); hasOption {
			value, valid := strings.CutPrefix(option, "show=")
			n, err := strconv.Atoi(value)
			if kind != "podcasts" || base != "play" || !valid || err != nil || n < 1 {
				return fail("expected podcasts:play:show=N (one-based)")
			}
			action, showIndex = base, n-1
		}
		if action == "play" && (space == SpaceWatch || space == SpaceShorts) {
			return fail("use show for Watch and Shorts")
		}
		if action != "play" && action != "show" && action != "scroll" {
			return fail("unknown action")
		}
		if !seq.entered {
			// A dock click in the current space does not switch or pause first.
			if ui.spaces.ActiveSpace != space || ui.showingUtility {
				ui.switchSpace(space)
			}
			seq.entered = true
		}
		if action == "scroll" {
			ui.spaces.State(space).ScrollList.ScrollBy(scrollRows)
		}
		if action == "play" {
			state := ui.spaces.State(space)
			if showIndex >= 0 {
				if now.Sub(seq.started) > 45*time.Second {
					return fail("chosen show or episodes not loaded within 45 seconds")
				}
				if !seq.showRequested {
					if showIndex >= len(state.Shows) {
						return ""
					}
					state.Episodes = nil
					ui.loadPodcastEpisodes(state.Shows[showIndex], true)
					seq.started = now
					seq.showRequested = true
					return ""
				}
				if ui.podcastsLoading || len(state.Episodes) == 0 {
					return ""
				}
				ui.selectPodcastEpisode(state.Episodes[0], state.Episodes)
			} else if state.ResumeItem != nil {
				if space == SpaceMusic {
					ui.playMusicDock()
				} else {
					ui.playPodcastsDock()
				}
			} else if space == SpaceMusic && len(state.Tracks) > 0 {
				ui.selectMusicTrack(state.Tracks[0], state.Tracks)
			} else if space == SpacePodcasts && len(state.Episodes) > 0 {
				ui.selectPodcastEpisode(state.Episodes[0], state.Episodes)
			} else {
				if now.Sub(seq.started) > 45*time.Second {
					return fail("no real item loaded within 45 seconds")
				}
				return ""
			}
		}
	}
	seq.index++
	seq.done = seq.index == len(seq.steps)
	seq.entered = false
	seq.showRequested = false
	seq.started = time.Time{}
	return step
}

// Log after layout so the rectangle describes the space that was just drawn.
func (ui *UI) logDebugSequenceStep(step string) {
	if step == "" {
		return
	}
	st := embed.Status{Paused: ui.watchPaused, Muted: ui.watchMuted}
	if ui.embedPlayer != nil {
		st = ui.embedPlayer.Status()
	}
	state := ui.spaces.Active()
	dockID, dockTitle := "", ""
	dockPos := 0.0
	if state.ResumeItem != nil {
		dockID, dockTitle = state.ResumeItem.ID, state.ResumeItem.Title
		dockPos = ui.spaceItemPosition(state.ID, dockID)
	}
	fmt.Fprintf(os.Stderr, "debug-sequence step=%q active=%s owner=%s/%s dock=%s title=%q rect=%v mute=%t pause=%t playing=%t position=%.3f dock_position=%.3f\n",
		step, ui.spaces.ActiveSpace, ui.playerOwnerSpace, ui.playerOwnerID, dockID, dockTitle,
		ui.activeVideoBox, st.Muted, st.Paused, st.IsPlaying, st.TimePos, dockPos)
}
