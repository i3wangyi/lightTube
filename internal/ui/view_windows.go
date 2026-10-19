//go:build windows

// UNTESTED: The Windows UI view backend is untested on actual Windows hardware.
// It handles app.Win32ViewEvent and creates the Windows embed player.

package ui

import (
	"errors"
	"unsafe"

	"gioui.org/app"
	"lighttube/internal/embed"
)

type platformView struct {
	hwnd  uintptr
	valid bool
}

func (pv *platformView) Valid() bool {
	return pv.valid
}

func (pv *platformView) Display() unsafe.Pointer {
	return nil
}

func (pv *platformView) Surface() unsafe.Pointer {
	return nil
}

func (ui *UI) handleViewEvent(e any) bool {
	we, ok := e.(app.Win32ViewEvent)
	if !ok {
		return false
	}
	if we.Valid() {
		ui.view = platformView{
			hwnd:  we.HWND,
			valid: true,
		}
	} else {
		ui.resetEmbedPlayer()
		ui.view = platformView{}
	}
	return true
}

func (ui *UI) resetEmbedPlayer() {
	if ui.embedPlayer != nil {
		ui.embedPlayer.Stop()
		ui.embedPlayer.Close()
		ui.embedPlayer = nil
	}
}

func (ui *UI) createEmbedPlayer() (*embed.Player, error) {
	if !ui.view.valid || ui.view.hwnd == 0 {
		return nil, errors.New("embed: no valid win32 parent window")
	}
	return embed.NewWin32(ui.view.hwnd)
}
