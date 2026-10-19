//go:build linux && cgo

package embed

/*
#include "hwcaps.h"
*/
import "C"
import (
	"errors"
)

// DecodeCaps probes the GPU via VA-API for hardware decode capabilities.
func DecodeCaps() (Caps, error) {
	var cCaps C.va_decode_caps_t
	var errBuf [256]C.char

	ret := C.query_decode_caps(&cCaps, &errBuf[0], C.size_t(len(errBuf)))
	if ret != 0 {
		return Caps{}, errors.New(C.GoString(&errBuf[0]))
	}

	return Caps{
		H264:   cCaps.h264 != 0,
		HEVC:   cCaps.hevc != 0,
		VP9:    cCaps.vp9 != 0,
		AV1:    cCaps.av1 != 0,
		Driver: C.GoString(&cCaps.driver[0]),
	}, nil
}

// CheckLibmpv tests if libmpv.so.2 can be dynamically loaded and unloaded.
func CheckLibmpv() error {
	var errBuf [256]C.char
	ret := C.check_libmpv_loadable(&errBuf[0], C.size_t(len(errBuf)))
	if ret != 0 {
		return errors.New(C.GoString(&errBuf[0]))
	}
	return nil
}
