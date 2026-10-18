package ui

import (
	"gioui.org/io/key"

	"lighttube/internal/yt"
)

// Tab represents an active navigation tab.
type Tab int

const (
	TabSearch Tab = iota
	TabSubscriptions
	TabWatchLater
	TabHistory
)

func (t Tab) String() string {
	switch t {
	case TabSearch:
		return "Search"
	case TabSubscriptions:
		return "Subscriptions"
	case TabWatchLater:
		return "Watch later"
	case TabHistory:
		return "History"
	default:
		return "Search"
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
)

// ResolveKeyAction determines the high-level UI action for a key press event.
// It respects whether an editor currently holds focus.
func ResolveKeyAction(name key.Name, mods key.Modifiers, isEditorFocused bool) (KeyAction, Tab, int) {
	// Ctrl+1..4 switches tabs globally, even if an editor is focused.
	if mods.Contain(key.ModCtrl) {
		switch name {
		case "1":
			return ActionSwitchTab, TabSearch, 0
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

// State manages UI navigation, selection, and current item lists.
type State struct {
	CurrentTab    Tab
	Videos        []yt.Video
	SelectedIndex int
	Subscriptions []yt.Subscription
	Status        string
}

// NewState creates an initial UI state.
func NewState() *State {
	return &State{
		CurrentTab:    TabSearch,
		Videos:        nil,
		SelectedIndex: -1,
		Status:        "Ready",
	}
}

// SetVideos updates the active video list and adjusts selection.
func (s *State) SetVideos(videos []yt.Video) {
	s.Videos = videos
	if len(videos) == 0 {
		s.SelectedIndex = -1
		return
	}
	if s.SelectedIndex < 0 || s.SelectedIndex >= len(videos) {
		s.SelectedIndex = 0
	}
}

// MoveSelection adjusts the selected video index by delta, clamping to valid bounds.
func (s *State) MoveSelection(delta int) {
	if len(s.Videos) == 0 {
		s.SelectedIndex = -1
		return
	}

	newIdx := s.SelectedIndex + delta
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(s.Videos) {
		newIdx = len(s.Videos) - 1
	}
	s.SelectedIndex = newIdx
}

// SelectedVideo returns the currently selected video item, if any.
func (s *State) SelectedVideo() (yt.Video, bool) {
	if s.SelectedIndex < 0 || s.SelectedIndex >= len(s.Videos) {
		return yt.Video{}, false
	}
	return s.Videos[s.SelectedIndex], true
}

// SwitchTab changes the active tab and resets video selection.
func (s *State) SwitchTab(t Tab) {
	s.CurrentTab = t
	s.SelectedIndex = -1
}
