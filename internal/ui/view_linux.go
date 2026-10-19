//go:build linux

package ui

import (
	"errors"
	"unsafe"

	"gioui.org/app"
	"lighttube/internal/embed"
)

type viewType int

const (
	viewNone viewType = iota
	viewWayland
	viewX11
)

type platformView struct {
	vtype   viewType
	display unsafe.Pointer
	surface unsafe.Pointer
	window  uintptr
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
	switch ev := e.(type) {
	case app.WaylandViewEvent:
		if ev.Valid() {
			ui.view = platformView{
				vtype:   viewWayland,
				display: ev.Display,
				surface: ev.Surface,
				valid:   true,
			}
		} else {
			ui.resetEmbedPlayer()
			ui.view = platformView{}
		}
		return true

	case app.X11ViewEvent:
		if ev.Valid() {
			ui.view = platformView{
				vtype:   viewX11,
				display: ev.Display,
				window:  ev.Window,
				valid:   true,
			}
		} else {
			ui.resetEmbedPlayer()
			ui.view = platformView{}
		}
		return true
	}
	return false
}

func (ui *UI) resetEmbedPlayer() {
	if ui.embedPlayer != nil {
		ui.embedPlayer.Stop()
		ui.embedPlayer.Close()
		ui.embedPlayer = nil
	}
}

func (ui *UI) createEmbedPlayer() (*embed.Player, error) {
	switch ui.view.vtype {
	case viewWayland:
		return embed.NewWayland(ui.view.display, ui.view.surface)
	case viewX11:
		return embed.NewX11(ui.view.window)
	default:
		return nil, errors.New("embed: no active platform view")
	}
}
