//go:build !linux || !cgo

package embed

import (
	"image"
	"unsafe"
)

// Player is a stub player for unsupported platforms.
type Player struct{}

// New returns ErrUnsupported on unsupported platforms.
func New(display, surface unsafe.Pointer) (*Player, error) {
	return nil, ErrUnsupported
}

// Play returns ErrUnsupported on unsupported platforms.
func (p *Player) Play(opts PlayOptions) error {
	return ErrUnsupported
}

// Stop is a no-op on unsupported platforms.
func (p *Player) Stop() {}

// SetRect is a no-op on unsupported platforms.
func (p *Player) SetRect(rect image.Rectangle, scale int) {}

// SetHidden is a no-op on unsupported platforms.
func (p *Player) SetHidden(hidden bool) {}

// TogglePause is a no-op on unsupported platforms.
func (p *Player) TogglePause() {}

// SetPause is a no-op on unsupported platforms.
func (p *Player) SetPause(paused bool) {}

// Seek is a no-op on unsupported platforms.
func (p *Player) Seek(seconds float64, relative bool) {}

// SetVolume is a no-op on unsupported platforms.
func (p *Player) SetVolume(volume float64) {}

// ToggleMute is a no-op on unsupported platforms.
func (p *Player) ToggleMute() {}

// Status returns an empty Status on unsupported platforms.
func (p *Player) Status() Status {
	return Status{}
}

// Close is a no-op on unsupported platforms.
func (p *Player) Close() {}
