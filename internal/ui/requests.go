package ui

import "lighttube/internal/yt"

type loadKind int

const (
	loadSearch loadKind = iota
	loadContent
	loadPlaylists
	loadChannels
	loadLibrary
	loadKinds
)

type spaceRequest struct {
	state    *SpaceState
	kind     loadKind
	token    uint64
	revision uint64
	gen      int64
}

// Requests start and finish on the UI goroutine. Workers only carry the token.
func (ui *UI) beginSpaceRequest(space SpaceID, kind loadKind) spaceRequest {
	state := ui.spaces.State(space)
	state.requests[kind]++
	if kind == loadSearch || kind == loadContent {
		// Search and chips replace the same content, so either supersedes both.
		state.contentRevision++
		switch space {
		case SpaceShorts:
			ui.shortsLoading = false
		case SpacePodcasts:
			ui.podcastsLoading = false
		case SpaceMusic:
			ui.musicLoading = false
		}
		state.ContentLoaded = true
		state.ContentGeneration = ui.currentGen()
	}
	return spaceRequest{state, kind, state.requests[kind], state.contentRevision, ui.currentGen()}
}

func (ui *UI) requestCurrent(r spaceRequest) bool {
	return r.gen == ui.currentGen() && ui.spaces.State(r.state.ID) == r.state &&
		r.state.requests[r.kind] == r.token &&
		(r.kind > loadContent || r.state.contentRevision == r.revision)
}

func (ui *UI) setWatchVideos(videos []yt.Video) {
	state := ui.spaces.State(SpaceWatch)
	state.Videos = append([]yt.Video(nil), videos...)
	state.ContentLoaded = true
	state.ContentGeneration = ui.currentGen()
	if ui.spaces.ActiveSpace == SpaceWatch {
		ui.state.SetVideos(videos)
	}
}

func (ui *UI) setSpaceStatus(state *SpaceState, status string) {
	state.Status = status
	if ui.spaces.ActiveSpace == state.ID && !ui.showingUtility {
		ui.state.Status = status
	}
}
