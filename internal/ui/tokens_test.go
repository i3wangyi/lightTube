package ui

import (
	"image/color"
	"testing"
)

func TestPhase10Tokens(t *testing.T) {
	tests := []struct {
		name string
		got  color.NRGBA
		want color.NRGBA
	}{
		{"Bg", DarkColors.Bg, color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff}},
		{"Surface", DarkColors.Surface, color.NRGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}},
		{"Raised", DarkColors.Raised, color.NRGBA{R: 0x27, G: 0x27, B: 0x27, A: 0xff}},
		{"Hover", DarkColors.Hover, color.NRGBA{R: 0x35, G: 0x35, B: 0x35, A: 0xff}},
		{"Border", DarkColors.Border, color.NRGBA{R: 0x38, G: 0x38, B: 0x38, A: 0xff}},
		{"Text", DarkColors.Text, color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff}},
		{"Text2", DarkColors.Text2, color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff}},
		{"Accent", DarkColors.Accent, color.NRGBA{R: 0xff, G: 0x00, B: 0x33, A: 0xff}},
		{"Action", DarkColors.Action, color.NRGBA{R: 0xc9, G: 0x00, B: 0x28, A: 0xff}},
		{"Overlay", DarkColors.Overlay, color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xb8}},
		{"AmbientWatch", DarkColors.AmbientWatch, color.NRGBA{R: 88, G: 126, B: 161, A: 71}},
		{"AmbientMusic", DarkColors.AmbientMusic, color.NRGBA{R: 80, G: 57, B: 134, A: 51}},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("token %s = %+v, want %+v", tc.name, tc.got, tc.want)
		}
	}
}
