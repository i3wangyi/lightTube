//go:build windows

// UNTESTED: The Windows embedded player backend is untested on actual Windows hardware.
// It compiles without cgo (CGO_ENABLED=0) and uses syscall.NewLazyDLL with SyscallN
// to interact with user32.dll and mpv-2.dll.

package embed

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	createWindowExW = user32.NewProc("CreateWindowExW")
	destroyWindow   = user32.NewProc("DestroyWindow")
	moveWindow      = user32.NewProc("MoveWindow")
	showWindow      = user32.NewProc("ShowWindow")

	mpvDLL              = syscall.NewLazyDLL("mpv-2.dll")
	mpvCreate           = mpvDLL.NewProc("mpv_create")
	mpvInitialize       = mpvDLL.NewProc("mpv_initialize")
	mpvTerminateDestroy = mpvDLL.NewProc("mpv_terminate_destroy")
	mpvSetOptionString  = mpvDLL.NewProc("mpv_set_option_string")
	mpvCommand          = mpvDLL.NewProc("mpv_command")
	mpvGetPropertyStr   = mpvDLL.NewProc("mpv_get_property_string")
	mpvFree             = mpvDLL.NewProc("mpv_free")
)

const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipSiblings = 0x04000000
	swHide         = 0
	swShow         = 5
)

type windowsBackend struct {
	mu     sync.Mutex
	parent uintptr
	child  uintptr
	mpv    uintptr
	active bool
}

// NewWin32 creates an embed Player using the parent HWND from Win32ViewEvent.
func NewWin32(hwnd uintptr) (*Player, error) {
	if hwnd == 0 {
		return nil, errors.New("embed: invalid win32 parent window handle")
	}

	if err := mpvDLL.Load(); err != nil {
		return nil, fmt.Errorf("embed: cannot load mpv-2.dll: %w", err)
	}

	// UNTESTED RISK: this child window belongs to the calling goroutine's OS
	// thread, which runs no message loop. Win32 links the input queues of a
	// parent and a child from different threads, so the Gio window can stop
	// responding. If so, create the child on Gio's window thread instead.
	className, err := syscall.UTF16PtrFromString("STATIC")
	if err != nil {
		return nil, err
	}

	// Create child HWND under the Gio window.
	child, _, callErr := syscall.SyscallN(
		createWindowExW.Addr(),
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		uintptr(wsChild|wsVisible|wsClipSiblings),
		0, 0, 1, 1,
		hwnd,
		0, 0, 0,
	)
	if child == 0 {
		return nil, fmt.Errorf("embed: CreateWindowExW failed: %v", callErr)
	}

	return newPlayer(&windowsBackend{
		parent: hwnd,
		child:  child,
	}), nil
}

// New is unsupported on Windows.
func New(display, surface unsafe.Pointer) (*Player, error) {
	return nil, ErrUnsupported
}

// NewWayland is unsupported on Windows.
func NewWayland(display, surface unsafe.Pointer) (*Player, error) {
	return nil, ErrUnsupported
}

// NewX11 is unsupported on Windows.
func NewX11(window uintptr) (*Player, error) {
	return nil, ErrUnsupported
}

// CheckLibmpv tests whether mpv-2.dll can be loaded on Windows.
func CheckLibmpv() error {
	if err := mpvDLL.Load(); err != nil {
		return fmt.Errorf("cannot load mpv-2.dll: %w", err)
	}
	return nil
}

// DecodeCaps returns unsupported on Windows (hardware decode is handled directly by mpv).
func DecodeCaps() (Caps, error) {
	return Caps{}, ErrUnsupported
}

func (b *windowsBackend) setOption(name, val string) error {
	cName, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	cVal, err := syscall.BytePtrFromString(val)
	if err != nil {
		return err
	}
	r, _, _ := syscall.SyscallN(
		mpvSetOptionString.Addr(),
		b.mpv,
		uintptr(unsafe.Pointer(cName)),
		uintptr(unsafe.Pointer(cVal)),
	)
	if int32(r) < 0 {
		return fmt.Errorf("mpv_set_option_string %s failed with code %d", name, int32(r))
	}
	return nil
}

func (b *windowsBackend) command(args ...string) error {
	if b.mpv == 0 {
		return errors.New("embed: mpv not initialized")
	}
	cargs := make([]*byte, len(args)+1)
	for i, a := range args {
		p, err := syscall.BytePtrFromString(a)
		if err != nil {
			return err
		}
		cargs[i] = p
	}
	cargs[len(args)] = nil

	r, _, _ := syscall.SyscallN(
		mpvCommand.Addr(),
		b.mpv,
		uintptr(unsafe.Pointer(&cargs[0])),
	)
	if int32(r) < 0 {
		return fmt.Errorf("mpv_command failed with code %d", int32(r))
	}
	return nil
}

func (b *windowsBackend) getPropertyString(prop string) string {
	if b.mpv == 0 {
		return ""
	}
	cProp, err := syscall.BytePtrFromString(prop)
	if err != nil {
		return ""
	}
	r, _, _ := syscall.SyscallN(
		mpvGetPropertyStr.Addr(),
		b.mpv,
		uintptr(unsafe.Pointer(cProp)),
	)
	if r == 0 {
		return ""
	}
	defer syscall.SyscallN(mpvFree.Addr(), r)

	// Read null-terminated C string into Go string
	var bytes []byte
	p := *(**byte)(unsafe.Pointer(&r))
	for *p != 0 {
		bytes = append(bytes, *p)
		p = (*byte)(unsafe.Add(unsafe.Pointer(p), 1))
	}
	return string(bytes)
}

func (b *windowsBackend) Play(opts PlayOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stopLocked()

	mpv, _, _ := syscall.SyscallN(mpvCreate.Addr())
	if mpv == 0 {
		return errors.New("embed: mpv_create failed")
	}
	b.mpv = mpv

	widStr := strconv.FormatUint(uint64(b.child), 10)
	_ = b.setOption("wid", widStr)
	_ = b.setOption("vo", "gpu")

	hwdec := opts.HWDec
	if hwdec == "" {
		hwdec = "auto-safe"
	}
	_ = b.setOption("hwdec", hwdec)
	// The default demuxer cache (150 MiB ahead, 50 MiB back) was most of the
	// RSS during playback; 64 MiB is still about 100 s of 1080p H.264.
	_ = b.setOption("demuxer-max-bytes", "64MiB")
	_ = b.setOption("demuxer-max-back-bytes", "16MiB")
	_ = b.setOption("osc", "no")
	_ = b.setOption("input-default-bindings", "no")
	_ = b.setOption("save-position-on-quit", "yes")

	if opts.Title != "" {
		_ = b.setOption("title", opts.Title)
	}
	if opts.YTDLFormat != "" {
		_ = b.setOption("ytdl-format", opts.YTDLFormat)
	}
	if opts.CookiesFile != "" {
		_ = b.setOption("ytdl-raw-options", "cookies="+opts.CookiesFile)
	}

	initRet, _, _ := syscall.SyscallN(mpvInitialize.Addr(), b.mpv)
	if int32(initRet) < 0 {
		b.stopLocked()
		return fmt.Errorf("embed: mpv_initialize failed: %d", int32(initRet))
	}

	syscall.SyscallN(showWindow.Addr(), b.child, swShow)

	if err := b.command("loadfile", opts.URL); err != nil {
		b.stopLocked()
		return fmt.Errorf("embed: failed to load URL: %w", err)
	}

	b.active = true
	return nil
}

func (b *windowsBackend) stopLocked() {
	if b.mpv != 0 {
		syscall.SyscallN(mpvTerminateDestroy.Addr(), b.mpv)
		b.mpv = 0
	}
	if b.child != 0 {
		syscall.SyscallN(showWindow.Addr(), b.child, swHide)
	}
	b.active = false
	runtime.GC()
	debug.FreeOSMemory()
}

func (b *windowsBackend) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopLocked()
}

func (b *windowsBackend) SetRect(rect image.Rectangle, scale int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.child == 0 || !b.active {
		return
	}

	w := rect.Dx()
	h := rect.Dy()
	x := rect.Min.X
	y := rect.Min.Y

	syscall.SyscallN(
		moveWindow.Addr(),
		b.child,
		uintptr(x),
		uintptr(y),
		uintptr(w),
		uintptr(h),
		1,
	)
}

func (b *windowsBackend) SetHidden(hidden bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.child == 0 || !b.active {
		return
	}

	if hidden {
		syscall.SyscallN(showWindow.Addr(), b.child, swHide)
		_ = b.command("set", "pause", "yes")
	} else {
		syscall.SyscallN(showWindow.Addr(), b.child, swShow)
	}
}

func (b *windowsBackend) TogglePause() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mpv != 0 && b.active {
		_ = b.command("cycle", "pause")
	}
}

func (b *windowsBackend) SetPause(paused bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mpv != 0 && b.active {
		val := "no"
		if paused {
			val = "yes"
		}
		_ = b.command("set", "pause", val)
	}
}

func (b *windowsBackend) Seek(seconds float64, relative bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mpv != 0 && b.active {
		mode := "absolute"
		if relative {
			mode = "relative"
		}
		_ = b.command("seek", fmt.Sprintf("%f", seconds), mode)
	}
}

func (b *windowsBackend) SetVolume(volume float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mpv != 0 && b.active {
		if volume < 0.0 {
			volume = 0.0
		}
		if volume > 100.0 {
			volume = 100.0
		}
		_ = b.command("set", "volume", fmt.Sprintf("%f", volume))
	}
}

func (b *windowsBackend) ToggleMute() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mpv != 0 && b.active {
		_ = b.command("cycle", "mute")
	}
}

func (b *windowsBackend) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mpv == 0 || !b.active {
		return Status{}
	}

	timeStr := b.getPropertyString("time-pos")
	durStr := b.getPropertyString("duration")
	pauseStr := b.getPropertyString("pause")
	muteStr := b.getPropertyString("mute")
	volStr := b.getPropertyString("volume")
	hwdecStr := b.getPropertyString("hwdec-current")
	vcodecStr := b.getPropertyString("video-codec")
	if vcodecStr == "" {
		vcodecStr = b.getPropertyString("video-format")
	}
	eofStr := b.getPropertyString("eof-reached")

	tPos, _ := strconv.ParseFloat(timeStr, 64)
	dur, _ := strconv.ParseFloat(durStr, 64)
	vol, _ := strconv.ParseFloat(volStr, 64)

	return Status{
		TimePos:      tPos,
		Duration:     dur,
		Paused:       pauseStr == "yes",
		Muted:        muteStr == "yes",
		Volume:       vol,
		HWDecCurrent: hwdecStr,
		VideoCodec:   vcodecStr,
		IsPlaying:    true,
		EOFReached:   eofStr == "yes",
	}
}

func (b *windowsBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stopLocked()

	if b.child != 0 {
		syscall.SyscallN(destroyWindow.Addr(), b.child)
		b.child = 0
	}
}
