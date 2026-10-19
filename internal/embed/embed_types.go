package embed

import "errors"

// ErrUnsupported indicates embedded playback is not supported on this platform or build.
var ErrUnsupported = errors.New("embed: embedded player is not supported on this platform")

// Caps describes hardware video decoding capabilities queried from the GPU.
type Caps struct {
	H264   bool
	HEVC   bool
	VP9    bool
	AV1    bool
	Driver string
}

// Status represents the current player playback status.
type Status struct {
	TimePos      float64
	Duration     float64
	Paused       bool
	Muted        bool
	Volume       float64
	HWDecCurrent string
	VideoCodec   string
	IsPlaying    bool
	EOFReached   bool
}

// PlayOptions describes parameters for launching an embedded playback session.
type PlayOptions struct {
	URL         string
	Title       string
	YTDLFormat  string
	CookiesFile string
	HWDec       string
	WidthPx     int
	HeightPx    int
	Scale       int
	Debug       bool
}
