package player

import "testing"

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
