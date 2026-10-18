package player

// Power represents the system power supply state.
type Power int

const (
	PowerAC Power = iota
	PowerBattery
)

func (p Power) String() string {
	switch p {
	case PowerBattery:
		return "Battery"
	default:
		return "AC"
	}
}

// Mode represents the playback media mode.
type Mode int

const (
	ModeVideo Mode = iota
	ModeAudio
)

func (m Mode) String() string {
	switch m {
	case ModeAudio:
		return "Audio"
	default:
		return "Video"
	}
}

const (
	formatAC      = "bv*[vcodec^=avc1][height<=1080][fps<=30]+ba/bv*[vcodec^=avc1][height<=1080]+ba/bv*[vcodec^=vp9][height<=720]+ba/b[height<=480]"
	formatBattery = "bv*[vcodec^=avc1][height<=720][fps<=30]+ba/bv*[vcodec^=avc1][height<=720]+ba/bv*[vcodec^=vp9][height<=480]+ba/b[height<=480]"
	formatAudio   = "ba"
)

// Format returns the yt-dlp format selection string based on power and mode.
func Format(power Power, mode Mode) string {
	if mode == ModeAudio {
		return formatAudio
	}
	if power == PowerBattery {
		return formatBattery
	}
	return formatAC
}
