//go:build (!linux || !cgo) && !windows

package embed

import (
	"unsafe"
)

// New returns ErrUnsupported on unsupported platforms.
func New(display, surface unsafe.Pointer) (*Player, error) {
	return nil, ErrUnsupported
}

// NewWayland returns ErrUnsupported on unsupported platforms.
func NewWayland(display, surface unsafe.Pointer) (*Player, error) {
	return nil, ErrUnsupported
}

// NewX11 returns ErrUnsupported on unsupported platforms.
func NewX11(window uintptr) (*Player, error) {
	return nil, ErrUnsupported
}

// NewWin32 returns ErrUnsupported on unsupported platforms.
func NewWin32(hwnd uintptr) (*Player, error) {
	return nil, ErrUnsupported
}

// DecodeCaps returns ErrUnsupported on unsupported platforms.
func DecodeCaps() (Caps, error) {
	return Caps{}, ErrUnsupported
}

// CheckLibmpv returns ErrUnsupported on unsupported platforms.
func CheckLibmpv() error {
	return ErrUnsupported
}
