package embed

import (
	"runtime"
	"testing"
)

func TestDecodeCaps(t *testing.T) {
	caps, err := DecodeCaps()
	if runtime.GOOS != "linux" {
		if err == nil {
			t.Fatalf("expected error on non-linux OS, got nil")
		}
		return
	}

	// On our Linux test machine, VA-API is installed and hardware decoding is supported.
	if err != nil {
		t.Logf("DecodeCaps returned error: %v (ignoring if machine has no DRI render node)", err)
		return
	}

	t.Logf("DecodeCaps: H264=%v, HEVC=%v, VP9=%v, AV1=%v, Driver=%q",
		caps.H264, caps.HEVC, caps.VP9, caps.AV1, caps.Driver)

	if !caps.H264 {
		t.Errorf("expected H264 decode capability on test machine")
	}
	if caps.Driver != "iHD" && caps.Driver != "i965" {
		t.Logf("driver is %q", caps.Driver)
	}
}

func TestCheckLibmpv(t *testing.T) {
	err := CheckLibmpv()
	if runtime.GOOS != "linux" {
		if err == nil {
			t.Fatalf("expected error on non-linux OS, got nil")
		}
		return
	}

	if err != nil {
		t.Fatalf("CheckLibmpv failed on Linux: %v", err)
	}
}
