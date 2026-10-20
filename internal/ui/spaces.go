package ui

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"lighttube/internal/yt"
)

func mergeVideos(primary, extra []yt.Video) []yt.Video {
	seen := make(map[string]bool, len(primary)+len(extra))
	merged := make([]yt.Video, 0, len(primary)+len(extra))
	for _, list := range [][]yt.Video{primary, extra} {
		for _, video := range list {
			if video.ID == "" || seen[video.ID] {
				continue
			}
			seen[video.ID] = true
			merged = append(merged, video)
		}
	}
	return merged
}

// SpaceID identifies one of the four intention-specific spaces.
type SpaceID int

const (
	SpaceWatch SpaceID = iota
	SpaceShorts
	SpacePodcasts
	SpaceMusic
)

func (s SpaceID) String() string {
	switch s {
	case SpaceWatch:
		return "Watch"
	case SpaceShorts:
		return "Shorts"
	case SpacePodcasts:
		return "Podcasts"
	case SpaceMusic:
		return "Music"
	default:
		return "Watch"
	}
}

// SpaceConfig holds metadata and visual configuration for a space.
type SpaceConfig struct {
	ID       SpaceID
	Label    string
	Title    string
	Subtitle string
	Badge    string
	Chips    []string
}

// GetSpaceConfig returns static configuration for the given space matching Main.dc.html.
func GetSpaceConfig(id SpaceID) SpaceConfig {
	switch id {
	case SpaceWatch:
		return SpaceConfig{
			ID:       SpaceWatch,
			Label:    "Watch",
			Title:    "Your Watch space",
			Subtitle: "Videos from your interests and the creators you follow.",
			Badge:    "Autoplay off",
			Chips:    []string{"For you", "Following", "Continue", "Saved"},
		}
	case SpaceShorts:
		return SpaceConfig{
			ID:       SpaceShorts,
			Label:    "Shorts",
			Title:    "A little discovery",
			Subtitle: "Short videos in their own space. Your Watch queue stays yours.",
			Badge:    "Sound off",
			Chips:    []string{"Following", "Saved", "Explore"},
		}
	case SpacePodcasts:
		return SpaceConfig{
			ID:       SpacePodcasts,
			Label:    "Podcasts",
			Title:    "Your shows, your pace",
			Subtitle: "Pick up the conversation without finding your place again.",
			Badge:    "Episode queue",
			Chips:    []string{"Your shows", "New episodes", "Continue"},
		}
	case SpaceMusic:
		return SpaceConfig{
			ID:       SpaceMusic,
			Label:    "Music",
			Title:    "Something for right now",
			Subtitle: "Tracks, albums, and collections. All in one place.",
			Badge:    "Visible player",
			Chips:    []string{"Liked", "Albums", "Saved"},
		}
	default:
		return GetSpaceConfig(SpaceWatch)
	}
}

// SpaceState maintains the independent UI state of a single space.
type SpaceState struct {
	ID                SpaceID
	Chip              int         // Active chip index
	Query             string      // Search query
	ScrollList        layout.List // List maintaining scroll position
	Selection         int         // Selected item index
	Queue             []yt.Video  // Queue belonging to this space
	ResumeItem        *yt.Video   // Hero / resume item
	Videos            []yt.Video  // Cached video items for this space
	ContentGeneration int64
	ContentLoaded     bool
	Status            string
	requests          [loadKinds]uint64
	contentRevision   uint64

	// Shorts specific state
	ShortIndex  int
	Shorts      []yt.Video
	ShortsError string

	// Podcasts specific state
	Shows            []yt.PodcastShow
	SelectedShow     *yt.PodcastShow
	Episodes         []yt.Video
	PodcastPlaylists []yt.Playlist
	SelectedPlaylist *yt.Playlist
	PodcastsError    string

	// Music specific state
	Album           *yt.Album
	Tracks          []yt.Video
	CurrentTrack    *yt.Video
	LastMusicAlbum  *yt.Album
	LastMusicTracks []yt.Video
}

// SpacesManager manages all 4 spaces and their switching.
type SpacesManager struct {
	ActiveSpace SpaceID
	Spaces      map[SpaceID]*SpaceState
	SpaceClicks map[SpaceID]*widget.Clickable
	ChipClicks  [8]widget.Clickable
	AmbientBtn  widget.Clickable

	// Shorts controls
	ShortsPrevClick    widget.Clickable
	ShortsNextClick    widget.Clickable
	ShortsSaveClick    widget.Clickable
	ShortsRelatedClick widget.Clickable
	ShortsStageClick   widget.Clickable
	ShortsSoundClick   widget.Clickable
	ShortsOpenWatchBtn widget.Clickable

	// Podcasts controls
	ShowClicks            []widget.Clickable
	EpisodeClicks         []widget.Clickable
	PodcastPlaylistClicks []widget.Clickable
	PodcastsDockPlay      widget.Clickable

	// Music controls
	MusicOpenColBtn widget.Clickable
	MusicSaveColBtn widget.Clickable
	TrackClicks     []widget.Clickable
	MusicDockPlay   widget.Clickable

	// Broad search ("All YouTube")
	AllYouTubeClick widget.Clickable
}

// NewSpacesManager creates a SpacesManager with all 4 spaces initialized.
func NewSpacesManager() *SpacesManager {
	sm := &SpacesManager{
		ActiveSpace: SpaceWatch,
		Spaces:      make(map[SpaceID]*SpaceState),
		SpaceClicks: make(map[SpaceID]*widget.Clickable),
	}
	for _, id := range []SpaceID{SpaceWatch, SpaceShorts, SpacePodcasts, SpaceMusic} {
		sm.Spaces[id] = newSpaceState(id, 0)
		sm.SpaceClicks[id] = new(widget.Clickable)
	}
	return sm
}

func newSpaceState(id SpaceID, chip int) *SpaceState {
	st := &SpaceState{ID: id, Chip: chip, Selection: -1}
	st.ScrollList.Axis = layout.Vertical
	return st
}

// ResetContent replaces every space's state with a fresh one, keeping only
// the chosen chip. Used at account boundaries so no field of the old
// account survives, including fields added later.
func (sm *SpacesManager) ResetContent() {
	for id, old := range sm.Spaces {
		sm.Spaces[id] = newSpaceState(id, old.Chip)
	}
}

// Active returns the SpaceState of the currently active space.
func (sm *SpacesManager) Active() *SpaceState {
	return sm.Spaces[sm.ActiveSpace]
}

// State returns the SpaceState for the specified space.
func (sm *SpacesManager) State(id SpaceID) *SpaceState {
	return sm.Spaces[id]
}
