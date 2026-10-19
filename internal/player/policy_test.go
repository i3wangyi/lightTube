package player

import (
	"runtime"
	"testing"

	"lighttube/internal/embed"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name  string
		power Power
		mode  Mode
		want  string
	}{
		{
			name:  "video on AC power",
			power: PowerAC,
			mode:  ModeVideo,
			want:  "bv*[vcodec^=avc1][height<=1080][fps<=30]+ba/bv*[vcodec^=avc1][height<=1080]+ba/bv*[vcodec^=vp9][height<=720]+ba/b[height<=480]",
		},
		{
			name:  "video on battery power",
			power: PowerBattery,
			mode:  ModeVideo,
			want:  "bv*[vcodec^=avc1][height<=720][fps<=30]+ba/bv*[vcodec^=avc1][height<=720]+ba/bv*[vcodec^=vp9][height<=480]+ba/b[height<=480]",
		},
		{
			name:  "audio on AC power",
			power: PowerAC,
			mode:  ModeAudio,
			want:  "ba",
		},
		{
			name:  "audio on battery power",
			power: PowerBattery,
			mode:  ModeAudio,
			want:  "ba",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.power, tt.mode)
			if got != tt.want {
				t.Errorf("Format(%v, %v) = %q; want %q", tt.power, tt.mode, got, tt.want)
			}
		})
	}
}

func TestFormatFor(t *testing.T) {
	capsNone := embed.Caps{}
	capsH264Only := embed.Caps{H264: true}
	capsVP9 := embed.Caps{H264: true, VP9: true}
	capsAV1 := embed.Caps{H264: true, AV1: true}
	capsAll := embed.Caps{H264: true, HEVC: true, VP9: true, AV1: true}

	tests := []struct {
		name   string
		power  Power
		mode   Mode
		policy CodecPolicy
		caps   embed.Caps
		want   string
	}{
		// Audio mode always returns "ba" regardless of policy, power, or caps.
		{
			name:   "audio old-gpu AC",
			power:  PowerAC,
			mode:   ModeAudio,
			policy: CodecPolicyOldGPU,
			caps:   capsAll,
			want:   "ba",
		},
		{
			name:   "audio any battery",
			power:  PowerBattery,
			mode:   ModeAudio,
			policy: CodecPolicyAny,
			caps:   capsAll,
			want:   "ba",
		},
		{
			name:   "audio auto AC",
			power:  PowerAC,
			mode:   ModeAudio,
			policy: CodecPolicyAuto,
			caps:   capsH264Only,
			want:   "ba",
		},

		// old-gpu policy
		{
			name:   "old-gpu AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyOldGPU,
			caps:   capsAll,
			want:   "bv*[vcodec^=avc1][height<=1080][fps<=30]+ba/bv*[vcodec^=avc1][height<=1080]+ba/bv*[vcodec^=vp9][height<=720]+ba/b[height<=480]",
		},
		{
			name:   "old-gpu Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyOldGPU,
			caps:   capsAll,
			want:   "bv*[vcodec^=avc1][height<=720][fps<=30]+ba/bv*[vcodec^=avc1][height<=720]+ba/bv*[vcodec^=vp9][height<=480]+ba/b[height<=480]",
		},

		// any policy
		{
			name:   "any AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyAny,
			caps:   capsNone,
			want:   "bv*[height<=1080]+ba/b[height<=1080]",
		},
		{
			name:   "any Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyAny,
			caps:   capsNone,
			want:   "bv*[height<=720]+ba/b[height<=720]",
		},

		// auto policy: no caps (error fallback to old-gpu)
		{
			name:   "auto no caps AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsNone,
			want:   "bv*[vcodec^=avc1][height<=1080][fps<=30]+ba/bv*[vcodec^=avc1][height<=1080]+ba/bv*[vcodec^=vp9][height<=720]+ba/b[height<=480]",
		},
		{
			name:   "auto no caps Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsNone,
			want:   "bv*[vcodec^=avc1][height<=720][fps<=30]+ba/bv*[vcodec^=avc1][height<=720]+ba/bv*[vcodec^=vp9][height<=480]+ba/b[height<=480]",
		},

		// auto policy: H.264 only
		{
			name:   "auto H264 only AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsH264Only,
			want:   formatAC,
		},
		{
			name:   "auto H264 only Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsH264Only,
			want:   formatBattery,
		},

		// auto policy: VP9 supported
		{
			name:   "auto VP9 AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsVP9,
			want:   "bv*[vcodec~='(?i)^(?:vp9|avc1)'][height<=1080]+ba/b[height<=1080]",
		},
		{
			name:   "auto VP9 Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsVP9,
			want:   "bv*[vcodec~='(?i)^(?:vp9|avc1)'][height<=720]+ba/b[height<=720]",
		},

		// auto policy: AV1 supported
		{
			name:   "auto AV1 AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsAV1,
			want:   "bv*[vcodec~='(?i)^(?:av01|avc1)'][height<=1080]+ba/b[height<=1080]",
		},
		{
			name:   "auto AV1 Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsAV1,
			want:   "bv*[vcodec~='(?i)^(?:av01|avc1)'][height<=720]+ba/b[height<=720]",
		},

		// auto policy: VP9 and AV1 supported
		{
			name:   "auto AV1+VP9 AC",
			power:  PowerAC,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsAll,
			want:   "bv*[vcodec~='(?i)^(?:av01|vp9|avc1)'][height<=1080]+ba/b[height<=1080]",
		},
		{
			name:   "auto AV1+VP9 Battery",
			power:  PowerBattery,
			mode:   ModeVideo,
			policy: CodecPolicyAuto,
			caps:   capsAll,
			want:   "bv*[vcodec~='(?i)^(?:av01|vp9|avc1)'][height<=720]+ba/b[height<=720]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatFor(tt.power, tt.mode, tt.policy, tt.caps)
			if got != tt.want {
				t.Errorf("FormatFor(%v, %v, %q, %+v) = %q; want %q",
					tt.power, tt.mode, tt.policy, tt.caps, got, tt.want)
			}
		})
	}
}

func TestPolicyAutoPicks(t *testing.T) {
	if got := PolicyAutoPicks(embed.Caps{}); got != CodecPolicyOldGPU {
		t.Errorf("PolicyAutoPicks(empty) = %q; want %q", got, CodecPolicyOldGPU)
	}
	if got := PolicyAutoPicks(embed.Caps{H264: true}); got != CodecPolicyOldGPU {
		t.Errorf("PolicyAutoPicks(h264) = %q; want %q", got, CodecPolicyOldGPU)
	}
	if got := PolicyAutoPicks(embed.Caps{H264: true, VP9: true}); got != CodecPolicyAuto {
		t.Errorf("PolicyAutoPicks(vp9) = %q; want %q", got, CodecPolicyAuto)
	}
	if got := PolicyAutoPicks(embed.Caps{H264: true, AV1: true}); got != CodecPolicyAuto {
		t.Errorf("PolicyAutoPicks(av1) = %q; want %q", got, CodecPolicyAuto)
	}
}

func TestHWDec(t *testing.T) {
	old := "auto-safe"
	if runtime.GOOS == "linux" {
		old = "vaapi"
	}
	cases := []struct {
		policy CodecPolicy
		caps   embed.Caps
		want   string
	}{
		{CodecPolicyOldGPU, embed.Caps{}, old},
		{CodecPolicyAuto, embed.Caps{H264: true}, old},
		{CodecPolicyAuto, embed.Caps{H264: true, VP9: true}, "auto-safe"},
		{CodecPolicyAny, embed.Caps{H264: true}, "auto-safe"},
	}
	for _, c := range cases {
		if got := HWDec(c.policy, c.caps); got != c.want {
			t.Errorf("HWDec(%q, %+v) = %q, want %q", c.policy, c.caps, got, c.want)
		}
	}
}
