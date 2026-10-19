//go:build linux && cgo

package embed

/*
#cgo LDFLAGS: -ldl
#include "x11.h"
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

type x11Backend struct {
	ctx    *C.x11_embed_context
	active bool
}

// NewX11 creates an embed Player using the X11 parent window ID.
func NewX11(window uintptr) (*Player, error) {
	if window == 0 {
		return nil, errors.New("embed: invalid x11 window")
	}

	ctx := C.x11_embed_context_create(C.ulong(window))
	if ctx == nil {
		return nil, errors.New("embed: failed to create x11 embed context")
	}

	return newPlayer(&x11Backend{ctx: ctx}), nil
}

func (b *x11Backend) Play(opts PlayOptions) error {
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

	ret := C.x11_embed_player_start(
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
		return errors.New("embed: failed to start x11 embedded playback")
	}

	b.active = true
	return nil
}

func (b *x11Backend) Stop() {
	if b.ctx != nil && b.active {
		C.x11_embed_player_stop(b.ctx)
		b.active = false
		runtime.GC()
		debug.FreeOSMemory()
	}
}

func (b *x11Backend) SetRect(rect image.Rectangle, scale int) {
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

	C.x11_embed_player_set_rect(b.ctx, C.int(x), C.int(y), C.int(w), C.int(h), C.int(scale))
}

func (b *x11Backend) SetHidden(hidden bool) {
	if b.ctx == nil || !b.active {
		return
	}

	C.x11_embed_player_set_hidden(b.ctx, C.bool(hidden))
}

func (b *x11Backend) TogglePause() {
	if b.ctx != nil && b.active {
		C.x11_embed_player_toggle_pause(b.ctx)
	}
}

func (b *x11Backend) SetPause(paused bool) {
	if b.ctx != nil && b.active {
		C.x11_embed_player_set_pause(b.ctx, C.bool(paused))
	}
}

func (b *x11Backend) Seek(seconds float64, relative bool) {
	if b.ctx != nil && b.active {
		C.x11_embed_player_seek(b.ctx, C.double(seconds), C.bool(relative))
	}
}

func (b *x11Backend) SetVolume(volume float64) {
	if b.ctx != nil && b.active {
		C.x11_embed_player_set_volume(b.ctx, C.double(volume))
	}
}

func (b *x11Backend) ToggleMute() {
	if b.ctx != nil && b.active {
		C.x11_embed_player_toggle_mute(b.ctx)
	}
}

func (b *x11Backend) Status() Status {
	if b.ctx == nil || !b.active {
		return Status{}
	}

	var cStat C.embed_status
	C.x11_embed_player_get_status(b.ctx, &cStat)

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

func (b *x11Backend) Close() {
	if b.ctx != nil {
		C.x11_embed_context_destroy(b.ctx)
		b.ctx = nil
		b.active = false
		runtime.GC()
		debug.FreeOSMemory()
	}
}
