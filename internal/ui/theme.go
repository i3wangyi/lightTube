package ui

import (
	"image/color"

	"gioui.org/widget/material"
)

// ThemeColors defines the color scheme for the dark theme.
type ThemeColors struct {
	Bg            color.NRGBA
	Surface       color.NRGBA
	SurfaceHover  color.NRGBA
	SurfaceSelect color.NRGBA
	Fg            color.NRGBA
	Subdued       color.NRGBA
	Accent        color.NRGBA
	Border        color.NRGBA
	BadgeBg       color.NRGBA
	BadgeFg       color.NRGBA
}

// DarkColors contains color definitions for the dark UI mode.
var DarkColors = ThemeColors{
	Bg:            color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xff},
	Surface:       color.NRGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff},
	SurfaceHover:  color.NRGBA{R: 0x28, G: 0x28, B: 0x28, A: 0xff},
	SurfaceSelect: color.NRGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff},
	Fg:            color.NRGBA{R: 0xfa, G: 0xfa, B: 0xfa, A: 0xff},
	Subdued:       color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff},
	Accent:        color.NRGBA{R: 0x3e, G: 0xa6, B: 0xff, A: 0xff},
	Border:        color.NRGBA{R: 0x2e, G: 0x2e, B: 0x2e, A: 0xff},
	BadgeBg:       color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xd0},
	BadgeFg:       color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
}

// NewDarkTheme initializes a material Theme configured with dark palette colors.
func NewDarkTheme() (*material.Theme, ThemeColors) {
	th := material.NewTheme()
	th.Palette = material.Palette{
		Bg:         DarkColors.Bg,
		Fg:         DarkColors.Fg,
		ContrastBg: DarkColors.Accent,
		ContrastFg: DarkColors.Fg,
	}
	return th, DarkColors
}
