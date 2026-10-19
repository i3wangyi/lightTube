package embed

import "errors"

// ErrUnsupported indicates embedded playback is not supported on this platform or build.
var ErrUnsupported = errors.New("embed: embedded player is not supported on this platform")

// Status represents the current player playback status.
type Status struct {
	TimePos      float64
	Duration     float64
	Paused       bool
	Muted        bool
	Volume       float64
	HWDecCurrent string
	IsPlaying    bool
	EOFReached   bool
}

// PlayOptions describes parameters for launching an embedded playback session.
type PlayOptions struct {
	URL         string
	Title       string
	YTDLFormat  string
	CookiesFile string
	WidthPx     int
	HeightPx    int
	Scale       int
	Debug       bool
}
