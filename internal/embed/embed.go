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
	"unsafe"
)

type waylandBackend struct {
	ctx    *C.embed_context
	active bool
}

// NewWayland creates an embed Player using the Wayland display and parent surface pointers.
func NewWayland(display, surface unsafe.Pointer) (*Player, error) {
	if display == nil || surface == nil {
		return nil, errors.New("embed: nil display or surface pointer")
	}

	ctx := C.embed_context_create(display, surface)
	if ctx == nil {
		return nil, errors.New("embed: failed to create wayland embed context")
	}

	return newPlayer(&waylandBackend{ctx: ctx}), nil
}

// New creates an embed Player using the Wayland display and parent surface pointers.
func New(display, surface unsafe.Pointer) (*Player, error) {
	return NewWayland(display, surface)
}

// NewWin32 is unsupported on Linux.
func NewWin32(hwnd uintptr) (*Player, error) {
	return nil, ErrUnsupported
}

func (b *waylandBackend) Play(opts PlayOptions) error {
	if b.ctx == nil {
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

	var cHWDec *C.char
	if opts.HWDec != "" {
		cHWDec = C.CString(opts.HWDec)
		defer C.free(unsafe.Pointer(cHWDec))
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
		b.ctx,
		cURL,
		cTitle,
		cFormat,
		cCookies,
		cHWDec,
		C.int(opts.WidthPx),
		C.int(opts.HeightPx),
		C.int(scale),
		cDebug,
	)

	if ret < 0 {
		return errors.New("embed: failed to start embedded playback")
	}

	b.active = true
	return nil
}

func (b *waylandBackend) Stop() {
	if b.ctx != nil && b.active {
		C.embed_player_stop(b.ctx)
		b.active = false
		runtime.GC()
		debug.FreeOSMemory()
	}
}

func (b *waylandBackend) SetRect(rect image.Rectangle, scale int) {
	if b.ctx == nil || !b.active {
		return
	}

	if scale <= 0 {
		scale = 1
	}

	w := rect.Dx()
	h := rect.Dy()
	x := rect.Min.X
	y := rect.Min.Y

	C.embed_player_set_rect(b.ctx, C.int(x), C.int(y), C.int(w), C.int(h), C.int(scale))
}

func (b *waylandBackend) SetHidden(hidden bool) {
	if b.ctx == nil || !b.active {
		return
	}

	C.embed_player_set_hidden(b.ctx, C.bool(hidden))
}

func (b *waylandBackend) TogglePause() {
	if b.ctx != nil && b.active {
		C.embed_player_toggle_pause(b.ctx)
	}
}

func (b *waylandBackend) SetPause(paused bool) {
	if b.ctx != nil && b.active {
		C.embed_player_set_pause(b.ctx, C.bool(paused))
	}
}

func (b *waylandBackend) Seek(seconds float64, relative bool) {
	if b.ctx != nil && b.active {
		C.embed_player_seek(b.ctx, C.double(seconds), C.bool(relative))
	}
}

func (b *waylandBackend) SetVolume(volume float64) {
	if b.ctx != nil && b.active {
		C.embed_player_set_volume(b.ctx, C.double(volume))
	}
}

func (b *waylandBackend) ToggleMute() {
	if b.ctx != nil && b.active {
		C.embed_player_toggle_mute(b.ctx)
	}
}

func (b *waylandBackend) Status() Status {
	if b.ctx == nil || !b.active {
		return Status{}
	}

	var cStat C.embed_status
	C.embed_player_get_status(b.ctx, &cStat)

	return Status{
		TimePos:      float64(cStat.time_pos),
		Duration:     float64(cStat.duration),
		Paused:       bool(cStat.paused),
		Muted:        bool(cStat.muted),
		Volume:       float64(cStat.volume),
		HWDecCurrent: C.GoString(&cStat.hwdec_current[0]),
		VideoCodec:   C.GoString(&cStat.video_codec[0]),
		IsPlaying:    bool(cStat.is_playing),
		EOFReached:   bool(cStat.eof_reached),
	}
}

func (b *waylandBackend) Close() {
	if b.ctx != nil {
		C.embed_context_destroy(b.ctx)
		b.ctx = nil
		b.active = false
		runtime.GC()
		debug.FreeOSMemory()
	}
}
