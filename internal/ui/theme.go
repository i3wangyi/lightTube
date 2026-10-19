package ui

import (
	"bytes"
	"image/color"
	"os"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/widget/material"
	fontapi "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"

	"lighttube/internal/platform"
)

// ThemeColors defines the color scheme for the dark theme matching DESIGN.md.
type ThemeColors struct {
	Bg             color.NRGBA
	Surface        color.NRGBA
	SurfaceHover   color.NRGBA
	Border         color.NRGBA
	BorderFocused  color.NRGBA
	Text           color.NRGBA
	Text2          color.NRGBA
	Red            color.NRGBA
	BadgeBg        color.NRGBA
	BadgeFg        color.NRGBA
	ChipActiveBg   color.NRGBA
	ChipActiveText color.NRGBA

	// Aliases for compatibility with earlier code
	Fg            color.NRGBA
	Subdued       color.NRGBA
	Accent        color.NRGBA
	SurfaceSelect color.NRGBA
}

// DarkColors contains color definitions for the dark UI mode from DESIGN.md.
var DarkColors = ThemeColors{
	Bg:             color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff},
	Surface:        color.NRGBA{R: 0x27, G: 0x27, B: 0x27, A: 0xff},
	SurfaceHover:   color.NRGBA{R: 0x3f, G: 0x3f, B: 0x3f, A: 0xff},
	Border:         color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff},
	BorderFocused:  color.NRGBA{R: 0x1c, G: 0x62, B: 0xb9, A: 0xff},
	Text:           color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff},
	Text2:          color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff},
	Red:            color.NRGBA{R: 0xff, G: 0x00, B: 0x00, A: 0xff},
	BadgeBg:        color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xcc}, // 80% alpha
	BadgeFg:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	ChipActiveBg:   color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff},
	ChipActiveText: color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff},

	// Backwards compatibility
	Fg:            color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff},
	Subdued:       color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff},
	Accent:        color.NRGBA{R: 0xff, G: 0x00, B: 0x00, A: 0xff},
	SurfaceSelect: color.NRGBA{R: 0x27, G: 0x27, B: 0x27, A: 0xff},
}

type rawFontFace struct {
	face *fontapi.Font
}

func (r rawFontFace) Face() *fontapi.Face {
	return fontapi.NewFace(r.face)
}

// loadExplicitFonts loads standard platform fonts without scanning all system fonts.
func loadExplicitFonts() []font.FontFace {
	paths := platform.FontFiles()

	var allFaces []font.FontFace
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		// In TTC font collections (such as NotoSansCJK-Regular.ttc with 10 regional faces),
		// opentype.ParseCollection parses all 10 faces into memory (costing ~185 MB heap).
		// Inspect loaders to parse only the single face needed (Noto Sans CJK SC).
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err == nil && len(lds) > 1 {
			nameTag := ot.MustNewTag("name")
			var targetLd *ot.Loader
			for _, ld := range lds {
				buf, err := ld.RawTable(nameTag)
				if err != nil {
					continue
				}
				names, _, err := tables.ParseName(buf)
				if err != nil {
					continue
				}
				if names.Name(1) == "Noto Sans CJK SC" || names.Name(4) == "Noto Sans CJK SC" {
					targetLd = ld
					break
				}
			}
			if targetLd == nil {
				targetLd = lds[0]
			}
			ft, err := fontapi.NewFont(targetLd)
			if err == nil {
				allFaces = append(allFaces, font.FontFace{
					Font: opentype.DescriptionToFont(ft.Describe()),
					Face: rawFontFace{face: ft},
				})
			}
			continue
		}

		faces, err := opentype.ParseCollection(data)
		if err != nil {
			continue
		}
		allFaces = append(allFaces, faces...)
	}
	return allFaces
}

// NewDarkTheme initializes a material Theme configured with dark palette colors
// and an explicit font collection, disabling full system font scanning for sub-500ms startup.
// When no explicit font files are found, it falls back to Gio's system font scan.
func NewDarkTheme() (*material.Theme, ThemeColors) {
	faces := loadExplicitFonts()
	var shaper *text.Shaper
	if len(faces) > 0 {
		shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	} else {
		shaper = text.NewShaper()
	}

	th := material.NewTheme()
	th.Shaper = shaper
	th.Palette = material.Palette{
		Bg:         DarkColors.Bg,
		Fg:         DarkColors.Text,
		ContrastBg: DarkColors.Red,
		ContrastFg: DarkColors.Text,
	}
	return th, DarkColors
}
