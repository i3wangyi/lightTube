//go:build !linux

package ui

import "unsafe"

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
