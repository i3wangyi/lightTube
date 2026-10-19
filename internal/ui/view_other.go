//go:build !linux && !windows

package ui

import (
	"unsafe"

	"lighttube/internal/embed"
)

type platformView struct{}

func (pv *platformView) Valid() bool {
	return false
}

func (pv *platformView) Display() unsafe.Pointer {
	return nil
}

func (pv *platformView) Surface() unsafe.Pointer {
	return nil
}

func (ui *UI) handleViewEvent(e any) bool {
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
	return nil, embed.ErrUnsupported
}
