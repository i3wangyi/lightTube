package player

import (
	"fmt"
	"runtime"
	"strings"

	"lighttube/internal/embed"
)

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

// CodecPolicy determines the video codec selection strategy.
type CodecPolicy string

const (
	CodecPolicyAuto   CodecPolicy = "auto"
	CodecPolicyOldGPU CodecPolicy = "old-gpu"
	CodecPolicyAny    CodecPolicy = "any"
)

// PolicyAutoPicks returns the concrete codec policy that 'auto' would select based on GPU decode caps.
func PolicyAutoPicks(caps embed.Caps) CodecPolicy {
	if caps.VP9 || caps.AV1 {
		return CodecPolicyAuto
	}
	return CodecPolicyOldGPU
}

// HWDec returns mpv's hwdec value. Where the policy resolves to old-gpu on
// Linux, use vaapi: that is the measured path on the Broadwell target.
func HWDec(policy CodecPolicy, caps embed.Caps) string {
	return hwdecFor(runtime.GOOS, policy, caps)
}

func hwdecFor(goos string, policy CodecPolicy, caps embed.Caps) string {
	if policy == CodecPolicyOldGPU || ((policy == CodecPolicyAuto || policy == "") && PolicyAutoPicks(caps) == CodecPolicyOldGPU) {
		if goos == "linux" {
			return "vaapi"
		}
	}
	return "auto-safe"
}

const (
	formatAC      = "bv*[vcodec^=avc1][height<=1080][fps<=30]+ba/bv*[vcodec^=avc1][height<=1080]+ba/bv*[vcodec^=vp9][height<=720]+ba/b[height<=480]"
	formatBattery = "bv*[vcodec^=avc1][height<=720][fps<=30]+ba/bv*[vcodec^=avc1][height<=720]+ba/bv*[vcodec^=vp9][height<=480]+ba/b[height<=480]"
	formatAudio   = "ba"
)

// Format returns the yt-dlp format selection string for callers that do not specify a policy (defaults to old-gpu).
func Format(power Power, mode Mode) string {
	return FormatFor(power, mode, CodecPolicyOldGPU, embed.Caps{})
}

// FormatFor builds the yt-dlp format selection string according to power, mode, codec policy, and GPU decode caps.
func FormatFor(power Power, mode Mode, policy CodecPolicy, caps embed.Caps) string {
	if mode == ModeAudio {
		return formatAudio
	}

	switch policy {
	case CodecPolicyOldGPU:
		if power == PowerBattery {
			return formatBattery
		}
		return formatAC

	case CodecPolicyAny:
		if power == PowerBattery {
			return "bv*[height<=720]+ba/b[height<=720]"
		}
		return "bv*[height<=1080]+ba/b[height<=1080]"

	case CodecPolicyAuto, "":
		// Without VP9 or AV1 decode (or without caps at all), auto is exactly
		// old-gpu: keep its fps<=30 rule, which matters most on such GPUs.
		if PolicyAutoPicks(caps) == CodecPolicyOldGPU {
			if power == PowerBattery {
				return formatBattery
			}
			return formatAC
		}

		height := 1080
		if power == PowerBattery {
			height = 720
		}

		var codecs []string
		if caps.AV1 {
			codecs = append(codecs, "av01")
		}
		if caps.VP9 {
			codecs = append(codecs, "vp9")
		}
		codecs = append(codecs, "avc1")

		pattern := strings.Join(codecs, "|")
		return fmt.Sprintf("bv*[vcodec~='(?i)^(?:%s)'][height<=%d]+ba/b[height<=%d]", pattern, height, height)

	default:
		return FormatFor(power, mode, CodecPolicyAuto, caps)
	}
}
