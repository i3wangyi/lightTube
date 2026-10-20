package ui

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"lighttube/internal/yt"
)

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
			Chips:    []string{"Explore", "Following", "Saved"},
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
			Chips:    []string{"Discover", "Albums", "Saved"},
		}
	default:
		return GetSpaceConfig(SpaceWatch)
	}
}

// SpaceState maintains the independent UI state of a single space.
type SpaceState struct {
	ID         SpaceID
	Chip       int         // Active chip index
	Query      string      // Search query
	ScrollList layout.List // List maintaining scroll position
	Selection  int         // Selected item index
	Queue      []yt.Video  // Queue belonging to this space
	ResumeItem *yt.Video   // Hero / resume item
	Videos     []yt.Video  // Cached video items for this space
}

// SpacesManager manages all 4 spaces and their switching.
type SpacesManager struct {
	ActiveSpace SpaceID
	Spaces      map[SpaceID]*SpaceState
	SpaceClicks map[SpaceID]*widget.Clickable
	ChipClicks  [8]widget.Clickable
	AmbientBtn  widget.Clickable
}

// NewSpacesManager creates a SpacesManager with all 4 spaces initialized.
func NewSpacesManager() *SpacesManager {
	sm := &SpacesManager{
		ActiveSpace: SpaceWatch,
		Spaces:      make(map[SpaceID]*SpaceState),
		SpaceClicks: make(map[SpaceID]*widget.Clickable),
	}
	for _, id := range []SpaceID{SpaceWatch, SpaceShorts, SpacePodcasts, SpaceMusic} {
		sm.Spaces[id] = &SpaceState{
			ID:        id,
			Chip:      0,
			Query:     "",
			Selection: -1,
			Queue:     nil,
			Videos:    nil,
		}
		sm.Spaces[id].ScrollList.Axis = layout.Vertical
		sm.SpaceClicks[id] = new(widget.Clickable)
	}
	return sm
}

// Active returns the SpaceState of the currently active space.
func (sm *SpacesManager) Active() *SpaceState {
	return sm.Spaces[sm.ActiveSpace]
}

// State returns the SpaceState for the specified space.
func (sm *SpacesManager) State(id SpaceID) *SpaceState {
	return sm.Spaces[id]
}
