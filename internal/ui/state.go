package ui

import (
	"gioui.org/io/key"

	"lighttube/internal/yt"
)

// Tab represents an active view or mini-guide navigation item.
type Tab int

const (
	TabHome Tab = iota
	TabSubscriptions
	TabWatchLater
	TabHistory
	TabSearch
	TabWatch
)

func (t Tab) String() string {
	switch t {
	case TabHome:
		return "Home"
	case TabSubscriptions:
		return "Subscriptions"
	case TabWatchLater:
		return "Watch later"
	case TabHistory:
		return "History"
	case TabSearch:
		return "Search"
	case TabWatch:
		return "Watch"
	default:
		return "Home"
	}
}

// KeyAction represents an action triggered by keyboard shortcuts.
type KeyAction int

const (
	ActionNone KeyAction = iota
	ActionPlay
	ActionPlayAudio
	ActionToggleWatchLater
	ActionFocusSearch
	ActionPasteURL
	ActionSwitchTab
	ActionMoveSelection
	ActionWatchTogglePause
	ActionWatchSeek
	ActionWatchToggleMute
	ActionWatchToggleFullscreen
	ActionWatchBack
	ActionWatchExternalMPV
)

// ResolveKeyAction determines the high-level UI action for a key press event.
// It respects whether an editor currently holds focus.
func ResolveKeyAction(name key.Name, mods key.Modifiers, isEditorFocused bool) (KeyAction, Tab, int) {
	// Ctrl+1..4 switches to the 4 mini guide items (Home, Subs, Later, History).
	if mods.Contain(key.ModCtrl) {
		switch name {
		case "1":
			return ActionSwitchTab, TabHome, 0
		case "2":
			return ActionSwitchTab, TabSubscriptions, 0
		case "3":
			return ActionSwitchTab, TabWatchLater, 0
		case "4":
			return ActionSwitchTab, TabHistory, 0
		case "L", "l":
			return ActionFocusSearch, 0, 0
		case "V", "v":
			// Ctrl+V when not in an editor pastes URL and plays.
			if !isEditorFocused {
				return ActionPasteURL, 0, 0
			}
		}
	}

	// When an editor is focused, plain letters and arrows are handled by the editor itself.
	if isEditorFocused {
		return ActionNone, 0, 0
	}

	switch name {
	case key.NameLeftArrow:
		return ActionMoveSelection, 0, -1
	case key.NameRightArrow:
		return ActionMoveSelection, 0, 1
	case key.NameUpArrow:
		return ActionMoveSelection, 0, -1
	case key.NameDownArrow:
		return ActionMoveSelection, 0, 1
	case key.NameReturn, key.NameEnter:
		return ActionPlay, 0, 0
	case "A", "a":
		return ActionPlayAudio, 0, 0
	case "W", "w":
		return ActionToggleWatchLater, 0, 0
	}

	return ActionNone, 0, 0
}

// ResolveWatchKeyAction handles keyboard shortcuts specific to the watch page.
func ResolveWatchKeyAction(name key.Name, mods key.Modifiers, isEditorFocused bool) (KeyAction, Tab, int) {
	if mods.Contain(key.ModCtrl) {
		switch name {
		case "1":
			return ActionSwitchTab, TabHome, 0
		case "2":
			return ActionSwitchTab, TabSubscriptions, 0
		case "3":
			return ActionSwitchTab, TabWatchLater, 0
		case "4":
			return ActionSwitchTab, TabHistory, 0
		case "L", "l":
			return ActionFocusSearch, 0, 0
		case "V", "v":
			if !isEditorFocused {
				return ActionPasteURL, 0, 0
			}
		}
	}

	if isEditorFocused {
		return ActionNone, 0, 0
	}

	switch name {
	case " ", "K", "k":
		return ActionWatchTogglePause, 0, 0
	case "J", "j":
		return ActionWatchSeek, 0, -10
	case "L", "l":
		return ActionWatchSeek, 0, 10
	case key.NameLeftArrow:
		return ActionWatchSeek, 0, -5
	case key.NameRightArrow:
		return ActionWatchSeek, 0, 5
	case "M", "m":
		return ActionWatchToggleMute, 0, 0
	case "F", "f":
		return ActionWatchToggleFullscreen, 0, 0
	case key.NameEscape:
		return ActionWatchBack, 0, 0
	case "O", "o":
		return ActionWatchExternalMPV, 0, 0
	}

	return ActionNone, 0, 0
}

// State manages UI navigation, selection, and current item lists.
type State struct {
	CurrentTab          Tab
	PreviousTab         Tab
	Videos              []yt.Video
	SelectedIndex       int
	Subscriptions       []yt.Subscription
	ActiveChannelFilter string
	Status              string
	WatchVideo          yt.Video
	UpNextVideos        []yt.Video
}

// NewState creates an initial UI state with Home as the default view.
func NewState() *State {
	return &State{
		CurrentTab:    TabHome,
		Videos:        nil,
		SelectedIndex: -1,
		Status:        "",
	}
}

// FilteredVideos returns videos filtered by active channel chip if on Home tab.
func (s *State) FilteredVideos() []yt.Video {
	if s.CurrentTab == TabHome && s.ActiveChannelFilter != "" {
		filtered := make([]yt.Video, 0, len(s.Videos))
		for _, v := range s.Videos {
			if v.ChannelID == s.ActiveChannelFilter {
				filtered = append(filtered, v)
			}
		}
		return filtered
	}
	return s.Videos
}

// SetVideos updates the active video list and adjusts selection.
func (s *State) SetVideos(videos []yt.Video) {
	s.Videos = videos
	filtered := s.FilteredVideos()
	if len(filtered) == 0 {
		s.SelectedIndex = -1
		return
	}
	if s.SelectedIndex < 0 || s.SelectedIndex >= len(filtered) {
		s.SelectedIndex = 0
	}
}

// MoveSelection adjusts the selected video index by delta, clamping to valid bounds.
func (s *State) MoveSelection(delta int) {
	videos := s.FilteredVideos()
	if len(videos) == 0 {
		s.SelectedIndex = -1
		return
	}

	newIdx := s.SelectedIndex + delta
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(videos) {
		newIdx = len(videos) - 1
	}
	s.SelectedIndex = newIdx
}

// MoveSelectionGrid moves selection in a grid with the given columns count.
func (s *State) MoveSelectionGrid(deltaX, deltaY, columns int) {
	if columns < 1 {
		columns = 1
	}
	s.MoveSelection(deltaX + deltaY*columns)
}

// SelectedVideo returns the currently selected video item, if any.
func (s *State) SelectedVideo() (yt.Video, bool) {
	videos := s.FilteredVideos()
	if s.SelectedIndex < 0 || s.SelectedIndex >= len(videos) {
		return yt.Video{}, false
	}
	return videos[s.SelectedIndex], true
}

// SwitchTab changes the active tab and resets video selection.
func (s *State) SwitchTab(t Tab) {
	s.CurrentTab = t
	s.SelectedIndex = -1
}
