//go:build linux

package ui

import (
	"unsafe"

	"gioui.org/app"
)

type platformView struct {
	display unsafe.Pointer
	surface unsafe.Pointer
	valid   bool
}

func (pv *platformView) Valid() bool {
	return pv.valid
}

func (pv *platformView) Display() unsafe.Pointer {
	return pv.display
}

func (pv *platformView) Surface() unsafe.Pointer {
	return pv.surface
}

func (ui *UI) handleViewEvent(e any) bool {
	we, ok := e.(app.WaylandViewEvent)
	if !ok {
		return false
	}
	if we.Valid() {
		ui.view = platformView{
			display: we.Display,
			surface: we.Surface,
			valid:   true,
		}
	} else {
		if ui.embedPlayer != nil {
			ui.embedPlayer.Stop()
			ui.embedPlayer.Close()
			ui.embedPlayer = nil
		}
		ui.view = platformView{}
	}
	return true
}
