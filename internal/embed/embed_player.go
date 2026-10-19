package embed

import (
	"errors"
	"image"
	"sync"
)

type playerBackend interface {
	Play(opts PlayOptions) error
	Stop()
	SetRect(rect image.Rectangle, scale int)
	SetHidden(hidden bool)
	TogglePause()
	SetPause(paused bool)
	Seek(seconds float64, relative bool)
	SetVolume(volume float64)
	ToggleMute()
	Status() Status
	Close()
}

// Player controls libmpv playback embedded in the application window.
type Player struct {
	mu      sync.Mutex
	backend playerBackend
}

func newPlayer(b playerBackend) *Player {
	return &Player{backend: b}
}

// Play starts playback of the given URL with mpv options.
func (p *Player) Play(opts PlayOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend == nil {
		return errors.New("embed: player context is closed")
	}
	return p.backend.Play(opts)
}

// Stop ends playback and tears down mpv resources.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.Stop()
	}
}

// SetRect updates the video rectangle (in physical pixel coordinates) and buffer scale.
func (p *Player) SetRect(rect image.Rectangle, scale int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.SetRect(rect, scale)
	}
}

// SetHidden hides or unhides the player window and pauses playback when hidden.
func (p *Player) SetHidden(hidden bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.SetHidden(hidden)
	}
}

// TogglePause toggles playback pause state.
func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.TogglePause()
	}
}

// SetPause sets playback pause state.
func (p *Player) SetPause(paused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.SetPause(paused)
	}
}

// Seek seeks playback by relative or absolute seconds.
func (p *Player) Seek(seconds float64, relative bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.Seek(seconds, relative)
	}
}

// SetVolume sets playback audio volume (0-100).
func (p *Player) SetVolume(volume float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.SetVolume(volume)
	}
}

// ToggleMute toggles audio mute state.
func (p *Player) ToggleMute() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.ToggleMute()
	}
}

// Status reads current playback status from libmpv.
func (p *Player) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend == nil {
		return Status{}
	}
	return p.backend.Status()
}

// Close destroys the embed context and all underlying resources.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.backend != nil {
		p.backend.Close()
		p.backend = nil
	}
}
