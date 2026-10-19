//go:build linux && cgo

package embed

/*
#cgo pkg-config: wayland-client wayland-egl egl
#cgo LDFLAGS: -ldl
#include "embed.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"image"
	"runtime"
	"runtime/debug"
	"sync"
	"unsafe"
)

// Player controls libmpv OpenGL rendering to a Wayland subsurface.
type Player struct {
	mu     sync.Mutex
	ctx    *C.embed_context
	active bool
}

// New creates an embed Player using the Wayland display and parent surface pointers.
func New(display, surface unsafe.Pointer) (*Player, error) {
	if display == nil || surface == nil {
		return nil, errors.New("embed: nil display or surface pointer")
	}

	ctx := C.embed_context_create(display, surface)
	if ctx == nil {
		return nil, errors.New("embed: failed to create wayland embed context")
	}

	return &Player{ctx: ctx}, nil
}

// Play starts playback of the given URL with mpv options.
func (p *Player) Play(opts PlayOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx == nil {
		return errors.New("embed: player context is closed")
	}

	cURL := C.CString(opts.URL)
	defer C.free(unsafe.Pointer(cURL))

	var cTitle *C.char
	if opts.Title != "" {
		cTitle = C.CString(opts.Title)
		defer C.free(unsafe.Pointer(cTitle))
	}

	var cFormat *C.char
	if opts.YTDLFormat != "" {
		cFormat = C.CString(opts.YTDLFormat)
		defer C.free(unsafe.Pointer(cFormat))
	}

	var cCookies *C.char
	if opts.CookiesFile != "" {
		cCookies = C.CString(opts.CookiesFile)
		defer C.free(unsafe.Pointer(cCookies))
	}

	cDebug := C.int(0)
	if opts.Debug {
		cDebug = C.int(1)
	}

	scale := opts.Scale
	if scale <= 0 {
		scale = 1
	}

	ret := C.embed_player_start(
		p.ctx,
		cURL,
		cTitle,
		cFormat,
		cCookies,
		C.int(opts.WidthPx),
		C.int(opts.HeightPx),
		C.int(scale),
		cDebug,
	)

	if ret < 0 {
		return errors.New("embed: failed to start embedded playback")
	}

	p.active = true
	return nil
}

// Stop ends playback and tears down mpv and EGL subsurface resources.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil && p.active {
		C.embed_player_stop(p.ctx)
		p.active = false
		runtime.GC()
		debug.FreeOSMemory()
	}
}

// SetRect updates the subsurface video rectangle (in physical pixel coordinates) and buffer scale.
func (p *Player) SetRect(rect image.Rectangle, scale int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx == nil || !p.active {
		return
	}

	if scale <= 0 {
		scale = 1
	}

	w := rect.Dx()
	h := rect.Dy()
	x := rect.Min.X
	y := rect.Min.Y

	C.embed_player_set_rect(p.ctx, C.int(x), C.int(y), C.int(w), C.int(h), C.int(scale))
}

// SetHidden unmaps or maps the subsurface and pauses playback when hidden.
func (p *Player) SetHidden(hidden bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx == nil || !p.active {
		return
	}

	C.embed_player_set_hidden(p.ctx, C.bool(hidden))
}

// TogglePause toggles playback pause state.
func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil && p.active {
		C.embed_player_toggle_pause(p.ctx)
	}
}

// SetPause sets playback pause state.
func (p *Player) SetPause(paused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil && p.active {
		C.embed_player_set_pause(p.ctx, C.bool(paused))
	}
}

// Seek seeks playback by relative or absolute seconds.
func (p *Player) Seek(seconds float64, relative bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil && p.active {
		C.embed_player_seek(p.ctx, C.double(seconds), C.bool(relative))
	}
}

// SetVolume sets playback audio volume (0-100).
func (p *Player) SetVolume(volume float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil && p.active {
		C.embed_player_set_volume(p.ctx, C.double(volume))
	}
}

// ToggleMute toggles audio mute state.
func (p *Player) ToggleMute() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil && p.active {
		C.embed_player_toggle_mute(p.ctx)
	}
}

// Status reads current playback status from libmpv.
func (p *Player) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx == nil || !p.active {
		return Status{}
	}

	var cStat C.embed_status
	C.embed_player_get_status(p.ctx, &cStat)

	return Status{
		TimePos:      float64(cStat.time_pos),
		Duration:     float64(cStat.duration),
		Paused:       bool(cStat.paused),
		Muted:        bool(cStat.muted),
		Volume:       float64(cStat.volume),
		HWDecCurrent: C.GoString(&cStat.hwdec_current[0]),
		IsPlaying:    bool(cStat.is_playing),
		EOFReached:   bool(cStat.eof_reached),
	}
}

// Close destroys the embed context and all underlying resources.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx != nil {
		C.embed_context_destroy(p.ctx)
		p.ctx = nil
		p.active = false
		runtime.GC()
		debug.FreeOSMemory()
	}
}
