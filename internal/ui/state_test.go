package ui

import (
	"testing"

	"gioui.org/io/key"

	"lighttube/internal/yt"
)

func TestStateSelection(t *testing.T) {
	st := NewState()
	if st.SelectedIndex != -1 {
		t.Errorf("expected initial selected index -1, got %d", st.SelectedIndex)
	}

	videos := []yt.Video{
		{ID: "v1", Title: "Video 1"},
		{ID: "v2", Title: "Video 2"},
		{ID: "v3", Title: "Video 3"},
	}

	st.SetVideos(videos)
	if st.SelectedIndex != 0 {
		t.Errorf("expected initial selection 0, got %d", st.SelectedIndex)
	}

	v, ok := st.SelectedVideo()
	if !ok || v.ID != "v1" {
		t.Errorf("expected v1 selected, got %+v", v)
	}

	// Move down.
	st.MoveSelection(1)
	if st.SelectedIndex != 1 {
		t.Errorf("expected selection 1, got %d", st.SelectedIndex)
	}

	// Move beyond max.
	st.MoveSelection(5)
	if st.SelectedIndex != 2 {
		t.Errorf("expected clamped selection 2, got %d", st.SelectedIndex)
	}

	// Move before 0.
	st.MoveSelection(-10)
	if st.SelectedIndex != 0 {
		t.Errorf("expected clamped selection 0, got %d", st.SelectedIndex)
	}

	// Move in grid (columns = 2)
	st.MoveSelectionGrid(1, 0, 2)
	if st.SelectedIndex != 1 {
		t.Errorf("expected selection 1 after grid move, got %d", st.SelectedIndex)
	}

	// Empty list.
	st.SetVideos(nil)
	if st.SelectedIndex != -1 {
		t.Errorf("expected selection -1 for empty list, got %d", st.SelectedIndex)
	}
	_, ok = st.SelectedVideo()
	if ok {
		t.Error("expected ok=false for empty list SelectedVideo")
	}
}

func TestResolveKeyAction(t *testing.T) {
	tests := []struct {
		name            string
		keyName         key.Name
		mods            key.Modifiers
		isEditorFocused bool
		expectedAction  KeyAction
		expectedTab     Tab
		expectedDelta   int
	}{
		{
			name:           "Ctrl+1 switches to TabHome",
			keyName:        "1",
			mods:           key.ModCtrl,
			expectedAction: ActionSwitchTab,
			expectedTab:    TabHome,
		},
		{
			name:           "Ctrl+2 switches to TabSubscriptions",
			keyName:        "2",
			mods:           key.ModCtrl,
			expectedAction: ActionSwitchTab,
			expectedTab:    TabSubscriptions,
		},
		{
			name:           "Ctrl+3 switches to TabWatchLater",
			keyName:        "3",
			mods:           key.ModCtrl,
			expectedAction: ActionSwitchTab,
			expectedTab:    TabWatchLater,
		},
		{
			name:           "Ctrl+4 switches to TabHistory",
			keyName:        "4",
			mods:           key.ModCtrl,
			expectedAction: ActionSwitchTab,
			expectedTab:    TabHistory,
		},
		{
			name:           "Ctrl+5 switches to TabAccount",
			keyName:        "5",
			mods:           key.ModCtrl,
			expectedAction: ActionSwitchTab,
			expectedTab:    TabAccount,
		},
		{
			name:           "Ctrl+L focuses search box",
			keyName:        "L",
			mods:           key.ModCtrl,
			expectedAction: ActionFocusSearch,
		},
		{
			name:            "Ctrl+L focuses search box even when editor focused",
			keyName:         "L",
			mods:            key.ModCtrl,
			isEditorFocused: true,
			expectedAction:  ActionFocusSearch,
		},
		{
			name:            "Ctrl+V pastes URL when not editor focused",
			keyName:         "V",
			mods:            key.ModCtrl,
			isEditorFocused: false,
			expectedAction:  ActionPasteURL,
		},
		{
			name:            "Ctrl+V does nothing special when editor focused (handled by editor)",
			keyName:         "V",
			mods:            key.ModCtrl,
			isEditorFocused: true,
			expectedAction:  ActionNone,
		},
		{
			name:            "Up arrow moves selection up",
			keyName:         key.NameUpArrow,
			expectedAction:  ActionMoveSelection,
			expectedDelta:   -1,
			isEditorFocused: false,
		},
		{
			name:            "Down arrow moves selection down",
			keyName:         key.NameDownArrow,
			expectedAction:  ActionMoveSelection,
			expectedDelta:   1,
			isEditorFocused: false,
		},
		{
			name:            "Enter plays video",
			keyName:         key.NameEnter,
			expectedAction:  ActionPlay,
			isEditorFocused: false,
		},
		{
			name:            "Return plays video",
			keyName:         key.NameReturn,
			expectedAction:  ActionPlay,
			isEditorFocused: false,
		},
		{
			name:            "A plays audio",
			keyName:         "A",
			expectedAction:  ActionPlayAudio,
			isEditorFocused: false,
		},
		{
			name:            "W toggles watch later",
			keyName:         "W",
			expectedAction:  ActionToggleWatchLater,
			isEditorFocused: false,
		},
		{
			name:            "Letters in editor are ignored by list handler",
			keyName:         "A",
			isEditorFocused: true,
			expectedAction:  ActionNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action, tab, delta := ResolveKeyAction(tc.keyName, tc.mods, tc.isEditorFocused)
			if action != tc.expectedAction {
				t.Errorf("expected action %v, got %v", tc.expectedAction, action)
			}
			if tab != tc.expectedTab {
				t.Errorf("expected tab %v, got %v", tc.expectedTab, tab)
			}
			if delta != tc.expectedDelta {
				t.Errorf("expected delta %d, got %d", tc.expectedDelta, delta)
			}
		})
	}
}

func TestResolveWatchKeyAction(t *testing.T) {
	tests := []struct {
		name            string
		keyName         key.Name
		mods            key.Modifiers
		isEditorFocused bool
		expectedAction  KeyAction
		expectedDelta   int
	}{
		{
			name:           "Space pauses/resumes",
			keyName:        " ",
			expectedAction: ActionWatchTogglePause,
		},
		{
			name:           "K pauses/resumes",
			keyName:        "K",
			expectedAction: ActionWatchTogglePause,
		},
		{
			name:           "J seeks -10s",
			keyName:        "j",
			expectedAction: ActionWatchSeek,
			expectedDelta:  -10,
		},
		{
			name:           "L seeks +10s",
			keyName:        "L",
			expectedAction: ActionWatchSeek,
			expectedDelta:  10,
		},
		{
			name:           "Left arrow seeks -5s",
			keyName:        key.NameLeftArrow,
			expectedAction: ActionWatchSeek,
			expectedDelta:  -5,
		},
		{
			name:           "Right arrow seeks +5s",
			keyName:        key.NameRightArrow,
			expectedAction: ActionWatchSeek,
			expectedDelta:  5,
		},
		{
			name:           "M mutes",
			keyName:        "M",
			expectedAction: ActionWatchToggleMute,
		},
		{
			name:           "F toggles fullscreen",
			keyName:        "f",
			expectedAction: ActionWatchToggleFullscreen,
		},
		{
			name:           "Escape backs out",
			keyName:        key.NameEscape,
			expectedAction: ActionWatchBack,
		},
		{
			name:           "O opens external mpv",
			keyName:        "o",
			expectedAction: ActionWatchExternalMPV,
		},
		{
			name:            "Keys ignored when editor is focused",
			keyName:         "f",
			isEditorFocused: true,
			expectedAction:  ActionNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action, _, delta := ResolveWatchKeyAction(tc.keyName, tc.mods, tc.isEditorFocused)
			if action != tc.expectedAction {
				t.Errorf("expected action %v, got %v", tc.expectedAction, action)
			}
			if delta != tc.expectedDelta {
				t.Errorf("expected delta %d, got %d", tc.expectedDelta, delta)
			}
		})
	}
}
